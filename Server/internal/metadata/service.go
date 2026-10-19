package metadata

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
)

const (
	workerCount         = 3
	retryNotFound       = 7 * 24 * time.Hour
	retryError          = time.Hour
	refreshDetails      = 30 * 24 * time.Hour
	headshotsPerCredits = 10
	castPerTitle        = 20
	guestsPerEpisode    = 10
)

// Service matches files in "movie" and "tv" libraries to TMDB in the background and stores
// the results and artwork. Lookups never block callers: Enqueue only schedules work.
type Service struct {
	db     *database.DB
	notify func(path string) // called with a file/folder path when its metadata is ready

	mu       sync.Mutex
	client   *Client
	clientID string          // token+language the client was built with
	pending  map[string]bool // job IDs queued or running
	active   map[string]bool // "movie:123"/"tv:123" being written, protected from cleanup
	// Folders a discovery found not to be a single movie or a show with episodes, so browsing
	// doesn't list them again on every request.
	notTitleFolders map[string]time.Time
	jobs            chan job
	syncing         atomic.Bool

	newClient func(token, language string) *Client // replaceable in tests
}

type job struct {
	lookup  *database.Lookup // set for title lookups
	seasons []int            // tv lookups: seasons on disk to fetch once the show is matched
	// folder lookups: a title from the video files' own names, tried when the folder name finds
	// nothing
	fallback   *Ref
	showID     int // set for season fetches
	season     int
	notifyPath string
	// set for folder discovery: list folder to see whether it holds one movie or a show
	discoverLib    *config.MediaPath
	discoverFolder string
}

func (j job) id() string {
	if j.lookup != nil {
		return "lookup:" + j.lookup.Kind + ":" + j.lookup.LibraryID + ":" + j.lookup.QueryKey
	}
	if j.discoverLib != nil {
		return "folder:" + j.discoverLib.ID + ":" + j.discoverFolder
	}
	return fmt.Sprintf("season:%d:%d", j.showID, j.season)
}

func NewService(db *database.DB, notify func(path string)) *Service {
	return &Service{
		db:      db,
		notify:  notify,
		pending: map[string]bool{},
		active:  map[string]bool{},

		notTitleFolders: map[string]time.Time{},
		jobs:            make(chan job, 16384),

		newClient: NewClient,
	}
}

// Start runs the lookup workers until ctx is done.
func (s *Service) Start(ctx context.Context) {
	for i := 0; i < workerCount; i++ {
		go func() {
			for {
				select {
				case j := <-s.jobs:
					s.run(ctx, j)
				case <-ctx.Done():
					return
				}
			}
		}()
	}
}

// tmdb returns a client for the current configuration, or nil when no token is configured.
// Built lazily so a token added through Settings takes effect without a restart.
func (s *Service) tmdb() *Client {
	cfg := config.GlobalConfig.Metadata
	if cfg.TMDBAccessToken == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id := cfg.TMDBAccessToken + "|" + cfg.Language; s.client == nil || s.clientID != id {
		s.client, s.clientID = s.newClient(cfg.TMDBAccessToken, cfg.Language), id
	}
	return s.client
}

func artworkDir() string {
	return config.GlobalConfig.Media.ArtworkPath
}

// EnqueueRef schedules a lookup for ref's title (and its season, for episodes) if needed.
func (s *Service) EnqueueRef(ref Ref, notifyPath string) {
	l := &database.Lookup{Kind: ref.Kind, LibraryID: ref.LibraryID, QueryKey: ref.Key,
		QueryTitle: ref.Title, QueryYear: ref.Year, QueryCountry: ref.Country}
	var seasons []int
	if ref.Kind == config.LibraryTypeTV && (ref.IsEpisode || ref.Season > 0) {
		seasons = []int{ref.Season}
	}
	s.enqueue(job{lookup: l, seasons: seasons, notifyPath: notifyPath})
}

func (s *Service) enqueue(j job) {
	if s.tmdb() == nil {
		return
	}
	id := j.id()
	s.mu.Lock()
	if s.pending[id] {
		s.mu.Unlock()
		return
	}
	s.pending[id] = true
	s.mu.Unlock()
	select {
	case s.jobs <- j:
	default:
		// Queue full: drop it; the next browse or sync schedules it again.
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}
}

func (s *Service) run(ctx context.Context, j job) {
	defer func() {
		s.mu.Lock()
		delete(s.pending, j.id())
		s.mu.Unlock()
	}()
	client := s.tmdb()
	if client == nil {
		return
	}
	var err error
	if j.lookup != nil {
		err = s.runLookup(ctx, client, j)
	} else if j.discoverLib != nil {
		err = s.runDiscover(*j.discoverLib, j.discoverFolder)
	} else {
		err = s.runSeason(ctx, client, j.showID, j.season, j.notifyPath)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("[METADATA] %s: %v", j.id(), err)
	}
}

// due reports whether a previous attempt with this status should be retried now.
func due(status string, at time.Time) bool {
	switch status {
	case "not_found":
		return time.Since(at) > retryNotFound
	case "error":
		return time.Since(at) > retryError
	}
	return true
}

func (s *Service) runLookup(ctx context.Context, client *Client, j job) error {
	l := j.lookup
	existing, err := s.db.GetLookup(l.Kind, l.LibraryID, l.QueryKey)
	if err != nil {
		return err
	}
	if existing != nil && existing.Status != "matched" && (existing.Manual || !due(existing.Status, existing.AttemptedAt)) {
		return nil
	}

	tmdbID := 0
	if existing != nil && existing.Status == "matched" {
		tmdbID = existing.TMDBID
	} else {
		tmdbID, err = s.search(ctx, client, l)
		if err == nil && tmdbID == 0 && j.fallback != nil {
			alt := *l
			alt.QueryTitle, alt.QueryYear = j.fallback.Title, j.fallback.Year
			tmdbID, err = s.search(ctx, client, &alt)
		}
		l.AttemptedAt = time.Now()
		switch {
		case err != nil:
			l.Status = "error"
			s.db.SaveLookup(l)
			return err
		case tmdbID == 0:
			l.Status = "not_found"
			log.Printf("[METADATA] No TMDB match for %s %q (%d)", l.Kind, l.QueryTitle, l.QueryYear)
			return s.db.SaveLookup(l)
		}
		l.TMDBID, l.Status = tmdbID, "matched"
		if err := s.db.SaveLookup(l); err != nil {
			return err
		}
	}

	changed := false
	if l.Kind == config.LibraryTypeMovie {
		changed, err = s.ensureMovie(ctx, client, tmdbID)
	} else {
		changed, err = s.ensureShow(ctx, client, tmdbID)
	}
	if errors.Is(err, ErrNotFound) && !l.Manual {
		// The search result has no details record (removed from TMDB): treat as no match.
		// A manual match is kept as chosen.
		l.TMDBID, l.Status, l.AttemptedAt = 0, "not_found", time.Now()
		return s.db.SaveLookup(l)
	}
	if err != nil {
		return err
	}
	for _, season := range j.seasons {
		s.enqueue(job{showID: tmdbID, season: season, notifyPath: j.notifyPath})
	}
	if changed && s.notify != nil && j.notifyPath != "" {
		s.notify(j.notifyPath)
	}
	return nil
}

// search finds the best TMDB match for a lookup, or 0 if there is none.
func (s *Service) search(ctx context.Context, client *Client, l *database.Lookup) (int, error) {
	isTV := l.Kind == config.LibraryTypeTV
	find := func(year int) ([]SearchResult, error) {
		if isTV {
			return client.SearchTV(ctx, l.QueryTitle, year)
		}
		return client.SearchMovie(ctx, l.QueryTitle, year)
	}
	results, err := find(l.QueryYear)
	if err == nil && len(results) == 0 && l.QueryYear > 0 {
		// The folder's year may be off (re-release, festival date); try the title alone.
		results, err = find(0)
	}
	if err != nil {
		return 0, err
	}
	return pickResult(results, l.QueryTitle, l.QueryYear, l.QueryCountry, isTV), nil
}

// pickResult scores search results by title match, year and country. It requires a full or
// partial title match, except that TMDB's top result is accepted when its year matches exactly:
// TMDB's search also matches alternate titles ("Sorcerer's Stone" finds "Philosopher's Stone").
func pickResult(results []SearchResult, title string, year int, country string, isTV bool) int {
	want := NormalizeTitle(title)
	best, bestScore := 0, -100
	for i, r := range results {
		if i >= 10 {
			break
		}
		names, date := []string{r.Title, r.OriginalTitle}, r.ReleaseDate
		if isTV {
			names, date = []string{r.Name, r.OriginalName}, r.FirstAirDate
		}
		titleScore := 0
		for _, n := range names {
			titleScore = max(titleScore, titleSimilarity(want, NormalizeTitle(n)))
		}
		if titleScore == 0 {
			if i != 0 || year == 0 || yearOf(date) != year {
				continue
			}
		}
		score := titleScore * 10
		if ry := yearOf(date); year > 0 && ry > 0 {
			switch d := ry - year; {
			case d == 0:
				score += 6
			case d == 1 || d == -1:
				score += 3
			default:
				score -= 6
			}
		}
		if country != "" {
			for _, c := range r.OriginCountry {
				if c == country {
					score += 5
				}
			}
		}
		score -= i // earlier results are more relevant/popular
		if score > bestScore {
			best, bestScore = r.ID, score
		}
	}
	return best
}

// titleSimilarity scores two normalized titles: 3 for the same title (ignoring spaces, so
// "Tinkerbell" matches "Tinker Bell"), 1 for a partial match, 0 for unrelated titles.
func titleSimilarity(want, got string) int {
	switch {
	case want == "" || got == "":
		return 0
	case want == got || strings.ReplaceAll(want, " ", "") == strings.ReplaceAll(got, " ", ""):
		return 3
	case strings.Contains(got, want) || strings.Contains(want, got):
		return 1
	}
	// Same words apart from at most two extras: "Transformers 3 Dark of the Moon" vs
	// "Transformers: Dark of the Moon".
	a, b := strings.Fields(want), strings.Fields(got)
	if len(a) > len(b) {
		a, b = b, a
	}
	words := map[string]bool{}
	for _, w := range b {
		words[w] = true
	}
	for _, w := range a {
		if !words[w] {
			return 0
		}
	}
	if len(a) >= 2 && len(b)-len(a) <= 2 {
		return 1
	}
	return 0
}

func yearOf(date string) int {
	if len(date) >= 4 {
		y, _ := strconv.Atoi(date[:4])
		return y
	}
	return 0
}

// markActive protects an artwork folder from cleanup while it is being written.
func (s *Service) markActive(key string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on {
		s.active[key] = true
	} else {
		delete(s.active, key)
	}
}

// ensureMovie fetches and stores a movie's details and artwork unless they are fresh.
func (s *Service) ensureMovie(ctx context.Context, client *Client, id int) (bool, error) {
	if m, err := s.db.GetMovie(id); err != nil || movieFresh(m) {
		return false, err
	}
	s.markActive(fmt.Sprintf("movie:%d", id), true)
	defer s.markActive(fmt.Sprintf("movie:%d", id), false)

	d, err := client.Movie(ctx, id)
	if err != nil {
		return false, err
	}
	dir := fmt.Sprintf("movie/%d", id)
	m := &database.Movie{
		TMDBID: d.ID, Title: d.Title, OriginalTitle: d.OriginalTitle, Year: yearOf(d.ReleaseDate),
		ReleaseDate: d.ReleaseDate, Overview: d.Overview, Tagline: d.Tagline, Runtime: d.Runtime,
		Genres: genreList(d.Genres), IMDBID: d.IMDBID,
		Poster:    s.download(ctx, client, "w500", d.PosterPath, dir+"/poster"),
		Backdrop:  s.download(ctx, client, "w1280", d.BackdropPath, dir+"/backdrop"),
		Logo:      s.download(ctx, client, "w500", pickLogo(d.Images.Logos, client.language), dir+"/logo"),
		UpdatedAt: time.Now(),
	}

	var credits []database.Credit
	for i, c := range d.Credits.Cast {
		if i >= castPerTitle {
			break
		}
		credits = append(credits, database.Credit{Person: s.person(ctx, client, c, i < headshotsPerCredits),
			Role: "cast", Character: c.Character, Order: c.Order})
	}
	credits = append(credits, crewCredits(d.Credits.Crew)...)
	return true, s.db.SaveMovie(m, credits)
}

// ensureShow fetches and stores a show's details and artwork unless they are fresh.
func (s *Service) ensureShow(ctx context.Context, client *Client, id int) (bool, error) {
	sh, err := s.db.GetShow(id)
	if err != nil || showFresh(sh) {
		return false, err
	}
	if sh != nil && !artworkPresent(sh.Poster, sh.Backdrop) {
		// Its artwork folder is gone, episode stills included: fetch the seasons again too.
		if err := s.db.DeleteSeasons(id); err != nil {
			return false, err
		}
	}
	s.markActive(fmt.Sprintf("tv:%d", id), true)
	defer s.markActive(fmt.Sprintf("tv:%d", id), false)

	d, err := client.TV(ctx, id)
	if err != nil {
		return false, err
	}
	dir := fmt.Sprintf("tv/%d", id)
	runtime := 0
	if len(d.EpisodeRunTime) > 0 {
		runtime = d.EpisodeRunTime[0]
	}
	sh = &database.Show{
		TMDBID: d.ID, Name: d.Name, OriginalName: d.OriginalName, Year: yearOf(d.FirstAirDate),
		FirstAirDate: d.FirstAirDate, Overview: d.Overview, Tagline: d.Tagline, Genres: genreList(d.Genres),
		IMDBID: d.ExternalIDs.IMDBID, Runtime: runtime,
		Poster:    s.download(ctx, client, "w500", d.PosterPath, dir+"/poster"),
		Backdrop:  s.download(ctx, client, "w1280", d.BackdropPath, dir+"/backdrop"),
		Logo:      s.download(ctx, client, "w500", pickLogo(d.Images.Logos, client.language), dir+"/logo"),
		UpdatedAt: time.Now(),
	}

	var credits []database.Credit
	for i, c := range d.AggregateCredits.Cast {
		if i >= castPerTitle {
			break
		}
		character := ""
		if len(c.Roles) > 0 {
			character = c.Roles[0].Character
		}
		p := Person{ID: c.ID, Name: c.Name, ProfilePath: c.ProfilePath}
		credits = append(credits, database.Credit{Person: s.person(ctx, client, p, i < headshotsPerCredits),
			Role: "cast", Character: character, Order: c.Order})
	}
	for i, c := range d.CreatedBy {
		credits = append(credits, database.Credit{Person: s.person(ctx, client, c, false), Role: "creator", Order: i})
	}
	return true, s.db.SaveShow(sh, credits)
}

func (s *Service) runSeason(ctx context.Context, client *Client, showID, number int, notifyPath string) error {
	if existing, err := s.db.GetSeason(showID, number); err != nil {
		return err
	} else if existing != nil {
		fresh := existing.Status == "fetched" && time.Since(existing.FetchedAt) < refreshDetails
		if fresh || (existing.Status != "fetched" && !due(existing.Status, existing.FetchedAt)) {
			return nil
		}
	}
	s.markActive(fmt.Sprintf("tv:%d", showID), true)
	defer s.markActive(fmt.Sprintf("tv:%d", showID), false)

	d, err := client.Season(ctx, showID, number)
	if errors.Is(err, ErrNotFound) {
		return s.db.SaveSeasonStatus(showID, number, "not_found", time.Now())
	}
	if err != nil {
		s.db.SaveSeasonStatus(showID, number, "error", time.Now())
		return err
	}

	dir := fmt.Sprintf("tv/%d", showID)
	season := &database.Season{ShowID: showID, SeasonNumber: number, Name: d.Name, Overview: d.Overview,
		Poster:    s.download(ctx, client, "w500", d.PosterPath, fmt.Sprintf("%s/season%02d", dir, number)),
		Status:    "fetched",
		FetchedAt: time.Now(),
	}
	var episodes []database.Episode
	credits := map[int][]database.Credit{}
	for _, e := range d.Episodes {
		episodes = append(episodes, database.Episode{
			ShowID: showID, SeasonNumber: number, EpisodeNumber: e.EpisodeNumber, TMDBID: e.ID,
			Name: e.Name, Overview: e.Overview, AirDate: e.AirDate, Runtime: e.Runtime,
			Still:     s.download(ctx, client, "w300", e.StillPath, fmt.Sprintf("%s/s%02de%02d", dir, number, e.EpisodeNumber)),
			UpdatedAt: time.Now(),
		})
		c := crewCredits(e.Crew)
		for i, g := range e.GuestStars {
			if i >= guestsPerEpisode {
				break
			}
			c = append(c, database.Credit{Person: s.person(ctx, client, g, false), Role: "guest", Character: g.Character, Order: g.Order})
		}
		credits[e.EpisodeNumber] = c
	}
	if err := s.db.SaveSeason(season, episodes, credits); err != nil {
		return err
	}
	if s.notify != nil && notifyPath != "" {
		s.notify(notifyPath)
	}
	return nil
}

// movieFresh reports whether a stored movie needs no refresh: recent, with its artwork on disk.
func movieFresh(m *database.Movie) bool {
	return m != nil && time.Since(m.UpdatedAt) < refreshDetails && artworkPresent(m.Poster, m.Backdrop)
}

func showFresh(sh *database.Show) bool {
	return sh != nil && time.Since(sh.UpdatedAt) < refreshDetails && artworkPresent(sh.Poster, sh.Backdrop)
}

// artworkPresent reports whether every given (non-empty) artwork file exists.
func artworkPresent(rels ...string) bool {
	for _, rel := range rels {
		if rel == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(artworkDir(), filepath.FromSlash(rel))); err != nil {
			return false
		}
	}
	return true
}

// crewCredits keeps directors and writers, once per person and role.
func crewCredits(crew []Person) []database.Credit {
	var credits []database.Credit
	seen := map[string]bool{}
	for _, c := range crew {
		role := ""
		switch c.Job {
		case "Director":
			role = "director"
		case "Screenplay", "Writer", "Teleplay", "Story", "Novel", "Author":
			role = "writer"
		}
		key := role + ":" + strconv.Itoa(c.ID)
		if role == "" || seen[key] {
			continue
		}
		seen[key] = true
		credits = append(credits, database.Credit{Person: database.Person{TMDBID: c.ID, Name: c.Name}, Role: role, Order: len(credits)})
	}
	return credits
}

// person builds a credit's person, downloading their headshot when wanted and not yet stored.
func (s *Service) person(ctx context.Context, client *Client, p Person, headshot bool) database.Person {
	out := database.Person{TMDBID: p.ID, Name: p.Name}
	if !headshot || p.ProfilePath == "" {
		return out
	}
	if existing, err := s.db.PersonProfile(p.ID); err == nil && existing != "" {
		out.Profile = existing
		return out
	}
	out.Profile = s.download(ctx, client, "w185", p.ProfilePath, fmt.Sprintf("person/%d", p.ID))
	return out
}

// download saves a TMDB image under the artwork directory and returns its relative path, or ""
// if there is no image or it couldn't be fetched (metadata is still saved without it).
func (s *Service) download(ctx context.Context, client *Client, size, imagePath, relBase string) string {
	if imagePath == "" {
		return ""
	}
	rel := relBase + strings.ToLower(filepath.Ext(imagePath))
	if err := client.DownloadImage(ctx, size, imagePath, filepath.Join(artworkDir(), filepath.FromSlash(rel))); err != nil {
		log.Printf("[METADATA] Failed to download %s: %v", imagePath, err)
		return ""
	}
	return rel
}

// pickLogo chooses the best-rated logo in the configured language, else a language-neutral one.
func pickLogo(logos []Image, language string) string {
	lang := strings.SplitN(language, "-", 2)[0]
	best, bestScore := "", -1.0
	for _, l := range logos {
		score := l.VoteAverage
		switch {
		case l.Language != nil && *l.Language == lang:
			score += 100
		case l.Language == nil:
			score += 50
		default:
			continue
		}
		if score > bestScore {
			best, bestScore = l.FilePath, score
		}
	}
	return best
}

func genreList(genres []Genre) string {
	names := make([]string, 0, len(genres))
	for _, g := range genres {
		names = append(names, g.Name)
	}
	return strings.Join(names, ", ")
}

// SyncAll walks every typed library, schedules lookups for titles not yet matched, and removes
// metadata and artwork that no file uses any more. Only one sync runs at a time.
func (s *Service) SyncAll(ctx context.Context) {
	if s.tmdb() == nil || !s.syncing.CompareAndSwap(false, true) {
		return
	}
	defer s.syncing.Store(false)

	start := time.Now()
	complete := true
	for _, lib := range config.GlobalConfig.Media.Paths {
		if lib.Type == "" {
			continue
		}
		if err := s.syncLibrary(ctx, lib); err != nil {
			complete = false
			log.Printf("[METADATA] Sync of %q failed: %v", lib.Name, err)
		}
	}
	// Only prune artwork after every library was walked, or titles in an unreadable library
	// would lose theirs.
	if complete {
		s.cleanupArtwork()
	}
	log.Printf("[METADATA] Library sync finished in %s", time.Since(start).Round(time.Second))
}

func (s *Service) syncLibrary(ctx context.Context, lib config.MediaPath) error {
	root, err := filepath.Abs(lib.Path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(root); err != nil {
		return err
	}

	filesByFolder := map[string][]string{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped, not fatal
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path != root && skipEntry(d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && isVideo(d.Name()) {
			filesByFolder[filepath.Dir(path)] = append(filesByFolder[filepath.Dir(path)], path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	keepKeys, keepFolders := map[string]bool{}, map[string]bool{}
	refs := map[string]Ref{}
	fallbacks := map[string]*Ref{}
	seasons := map[string]map[int]bool{}
	notifyPaths := map[string]string{}

	showKeyCounts := map[string]map[string]int{} // tv: show folder -> episode key -> count
	for folder, files := range filesByFolder {
		if lib.Type == config.LibraryTypeMovie && folder != root {
			if folderKey := movieFolderKey(lib, files); folderKey != "" {
				keepFolders[folder] = true
				if err := s.db.SetFolderKey(lib.ID, folder, lib.Type, folderKey); err != nil {
					return err
				}
			}
		}
		for _, f := range files {
			ref, ok := ResolveFile(lib, f)
			// Only episodes name a show; other TV files (extras, "Week 3" recordings) don't.
			if !ok || (lib.Type == config.LibraryTypeTV && !ref.IsEpisode) {
				continue
			}
			keepKeys[ref.Key] = true
			if _, seen := refs[ref.Key]; !seen {
				refs[ref.Key] = ref
				notifyPaths[ref.Key] = f
				if alt, ok := fallbackFor(lib, f, ref); ok {
					fallbacks[ref.Key] = &alt
				}
			}
			if ref.IsEpisode {
				if seasons[ref.Key] == nil {
					seasons[ref.Key] = map[int]bool{}
				}
				seasons[ref.Key][ref.Season] = true
				if show := ShowFolderOf(lib, f); show != "" && show != f {
					if showKeyCounts[show] == nil {
						showKeyCounts[show] = map[string]int{}
					}
					showKeyCounts[show][ref.Key]++
				}
			}
		}
	}
	// Each TV show folder stands for the show most of its episodes name.
	for show, counts := range showKeyCounts {
		keepFolders[show] = true
		if err := s.db.SetFolderKey(lib.ID, show, lib.Type, mostCommonKey(counts)); err != nil {
			return err
		}
	}

	if err := s.db.PruneLibraryMetadata(lib.Type, lib.ID, keepKeys, keepFolders); err != nil {
		return err
	}

	queued := 0
	for key, ref := range refs {
		l := &database.Lookup{Kind: ref.Kind, LibraryID: lib.ID, QueryKey: key, QueryTitle: ref.Title,
			QueryYear: ref.Year, QueryCountry: ref.Country}
		var seasonList []int
		for n := range seasons[key] {
			seasonList = append(seasonList, n)
		}
		if !s.needsWork(l, seasonList) {
			continue
		}
		s.enqueue(job{lookup: l, seasons: seasonList, notifyPath: notifyPaths[key], fallback: fallbacks[key]})
		queued++
	}
	log.Printf("[METADATA] %s: %d titles on disk, %d queued for lookup", lib.Name, len(refs), queued)
	return nil
}

// needsWork reports whether a title (or one of its seasons) is missing or due for a retry, so a
// sync doesn't queue thousands of no-op jobs.
func (s *Service) needsWork(l *database.Lookup, seasons []int) bool {
	existing, err := s.db.GetLookup(l.Kind, l.LibraryID, l.QueryKey)
	if err != nil || existing == nil {
		return true
	}
	if existing.Status != "matched" {
		return !existing.Manual && due(existing.Status, existing.AttemptedAt)
	}
	if l.Kind == config.LibraryTypeMovie {
		m, err := s.db.GetMovie(existing.TMDBID)
		return err != nil || !movieFresh(m)
	}
	if sh, err := s.db.GetShow(existing.TMDBID); err != nil || !showFresh(sh) {
		return true
	}
	for _, n := range seasons {
		season, err := s.db.GetSeason(existing.TMDBID, n)
		if err != nil || season == nil {
			return true
		}
		if season.Status == "fetched" && time.Since(season.FetchedAt) > refreshDetails {
			return true
		}
		if season.Status != "fetched" && due(season.Status, season.FetchedAt) {
			return true
		}
	}
	return false
}

// rememberNotTitle is how long a folder found to hold no movie or show is skipped by
// browse-time discovery. A sync re-checks every folder regardless.
const rememberNotTitle = 10 * time.Minute

// DiscoverFolder schedules a background check of a folder no sync has recorded (new or renamed
// since): a movie folder holding videos of one title, or a TV show folder whose season folders
// hold episodes. If it qualifies, it is looked up. Browsing never waits for it.
func (s *Service) DiscoverFolder(lib config.MediaPath, folder string) {
	s.mu.Lock()
	checked, seen := s.notTitleFolders[folder]
	s.mu.Unlock()
	if seen && time.Since(checked) < rememberNotTitle {
		return
	}
	s.enqueue(job{discoverLib: &lib, discoverFolder: folder})
}

func (s *Service) runDiscover(lib config.MediaPath, folder string) error {
	var jobs []job
	var err error
	if lib.Type == config.LibraryTypeTV {
		jobs, err = s.discoverShow(lib, folder)
	} else {
		jobs, err = s.discoverMovies(lib, folder)
	}
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		s.mu.Lock()
		s.notTitleFolders[folder] = time.Now()
		s.mu.Unlock()
	}
	for _, j := range jobs {
		s.enqueue(j)
	}
	return nil
}

// discoverMovies looks up every video directly in a movie-library folder (each by its own file
// name), and records the folder as a movie folder when they are all the same title.
func (s *Service) discoverMovies(lib config.MediaPath, folder string) ([]job, error) {
	videos, err := videosIn(folder)
	if err != nil || len(videos) == 0 {
		return nil, err
	}
	if key := movieFolderKey(lib, videos); key != "" {
		if err := s.db.SetFolderKey(lib.ID, folder, lib.Type, key); err != nil {
			return nil, err
		}
	}
	var jobs []job
	seen := map[string]bool{}
	for _, v := range videos {
		ref, ok := ResolveFile(lib, v)
		if !ok || seen[ref.Key] {
			continue
		}
		seen[ref.Key] = true
		j := lookupJob(ref, v, nil)
		if alt, ok := fallbackFor(lib, v, ref); ok {
			j.fallback = &alt
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// discoverShow checks a TV show folder, and its immediate subfolders (season folders), for
// episode files.
// discoverShow looks up the shows named by the episode files in a TV show folder and its
// immediate subfolders (season folders), and records the folder as the most common one.
func (s *Service) discoverShow(lib config.MediaPath, folder string) ([]job, error) {
	folders := []string{folder}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() && !skipEntry(e) {
			folders = append(folders, filepath.Join(folder, e.Name()))
		}
	}
	shows := newShowCollector()
	for _, f := range folders {
		videos, _ := videosIn(f)
		for _, v := range videos {
			if ref, ok := ResolveFile(lib, v); ok && ref.IsEpisode {
				shows.add(lib, v, ref)
			}
		}
	}
	if len(shows.refs) == 0 {
		return nil, nil
	}
	if err := s.db.SetFolderKey(lib.ID, folder, lib.Type, mostCommonKey(shows.counts)); err != nil {
		return nil, err
	}
	return shows.jobs(), nil
}

// showCollector gathers the shows, seasons and fallbacks seen among a folder's episodes.
type showCollector struct {
	refs      map[string]Ref
	seasons   map[string]map[int]bool
	counts    map[string]int
	files     map[string]string
	fallbacks map[string]*Ref
}

func newShowCollector() *showCollector {
	return &showCollector{refs: map[string]Ref{}, seasons: map[string]map[int]bool{}, counts: map[string]int{},
		files: map[string]string{}, fallbacks: map[string]*Ref{}}
}

func (c *showCollector) add(lib config.MediaPath, file string, ref Ref) {
	if _, seen := c.refs[ref.Key]; !seen {
		c.refs[ref.Key], c.files[ref.Key] = ref, file
		c.seasons[ref.Key] = map[int]bool{}
		if alt, ok := fallbackFor(lib, file, ref); ok {
			c.fallbacks[ref.Key] = &alt
		}
	}
	c.seasons[ref.Key][ref.Season] = true
	c.counts[ref.Key]++
}

func (c *showCollector) jobs() []job {
	var jobs []job
	for key, ref := range c.refs {
		var seasons []int
		for n := range c.seasons[key] {
			seasons = append(seasons, n)
		}
		j := lookupJob(ref, c.files[key], seasons)
		j.fallback = c.fallbacks[key]
		jobs = append(jobs, j)
	}
	return jobs
}

func lookupJob(ref Ref, notifyPath string, seasons []int) job {
	l := &database.Lookup{Kind: ref.Kind, LibraryID: ref.LibraryID, QueryKey: ref.Key,
		QueryTitle: ref.Title, QueryYear: ref.Year, QueryCountry: ref.Country}
	return job{lookup: l, seasons: seasons, notifyPath: notifyPath}
}

// videosIn lists the video files directly inside folder.
func videosIn(folder string) ([]string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	var videos []string
	for _, e := range entries {
		if !e.IsDir() && !skipEntry(e) && isVideo(e.Name()) {
			videos = append(videos, filepath.Join(folder, e.Name()))
		}
	}
	return videos, nil
}

// fallbackFor returns a second title to search when the primary one finds nothing:
//   - movies: the file's folder name (files are matched by their own name first)
//   - tv: whichever of the episode file's show name and its folder's show name the lookup
//     didn't use
func fallbackFor(lib config.MediaPath, file string, ref Ref) (Ref, bool) {
	root, _ := filepath.Abs(lib.Path)
	var alt Ref
	switch lib.Type {
	case config.LibraryTypeMovie:
		parent := filepath.Dir(file)
		if parent == root {
			return Ref{}, false
		}
		title, year := ParseTitleYear(filepath.Base(parent))
		alt = Ref{Kind: lib.Type, LibraryID: lib.ID, Key: LookupKey(title, year), Title: title, Year: year}
	case config.LibraryTypeTV:
		primary, fallback, ok := episodeShow(lib, root, file)
		switch {
		case !ok:
			return Ref{}, false
		case primary.Key != ref.Key:
			// ref came from the show folder (discovery): the episode's own resolution is the fallback
			alt = *primary
		case fallback != nil:
			alt = *fallback
		default:
			return Ref{}, false
		}
	default:
		return Ref{}, false
	}
	if alt.Title == "" || NormalizeTitle(alt.Title) == NormalizeTitle(ref.Title) {
		return Ref{}, false
	}
	return alt, true
}

// movieFolderKey returns the lookup key of a folder whose videos are all the same title (by file
// name), so the folder can show that movie. Folders of different titles return "".
func movieFolderKey(lib config.MediaPath, files []string) string {
	key := ""
	for _, f := range files {
		ref, ok := ResolveFile(lib, f)
		if !ok || (key != "" && ref.Key != key) {
			return ""
		}
		key = ref.Key
	}
	return key
}

// cleanupArtwork removes movies and shows no file is matched to any more: their artwork folders
// and their records (so matching one again later re-downloads everything).
func (s *Service) cleanupArtwork() {
	for _, kind := range []string{config.LibraryTypeMovie, config.LibraryTypeTV} {
		matched, err := s.db.MatchedTMDBIDs(kind)
		if err != nil {
			log.Printf("[METADATA] Artwork cleanup skipped: %v", err)
			return
		}
		// Candidates: artwork folders on disk and stored records.
		unused := map[int]bool{}
		dir := filepath.Join(artworkDir(), kind)
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if id, err := strconv.Atoi(e.Name()); err == nil && e.IsDir() && !matched[id] {
					unused[id] = true
				}
			}
		}
		if ids, err := s.db.TitleIDs(kind); err == nil {
			for _, id := range ids {
				if !matched[id] {
					unused[id] = true
				}
			}
		}

		removed := 0
		for id := range unused {
			s.mu.Lock()
			busy := s.active[fmt.Sprintf("%s:%d", kind, id)]
			s.mu.Unlock()
			if busy {
				continue
			}
			if err := os.RemoveAll(filepath.Join(dir, strconv.Itoa(id))); err != nil {
				continue
			}
			if err := s.db.DeleteTitle(kind, id); err != nil {
				log.Printf("[METADATA] Failed to remove %s %d: %v", kind, id, err)
				continue
			}
			removed++
		}
		if removed > 0 {
			log.Printf("[METADATA] Removed %d unused %s titles and their artwork", removed, kind)
		}
	}
}
