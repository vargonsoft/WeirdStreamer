package config

import "testing"

func TestInGuideFiller(t *testing.T) {
	old := GlobalConfig
	defer func() { GlobalConfig = old }()
	GlobalConfig = &Config{}
	GlobalConfig.Media.Paths = []MediaPath{
		{Name: "Movies", Path: "/media/movies"},
		{Name: "Commercials", Path: "/media/commercials", GuideFiller: true},
	}
	if !InGuideFiller("/media/commercials/1986/Pringles.mp4") {
		t.Fatal("a commercial isn't filler")
	}
	if InGuideFiller("/media/movies/Annie (1982)/Annie.mkv") || InGuideFiller("/media/commercials-old/x.mp4") {
		t.Fatal("a movie (or a look-alike folder) counted as filler")
	}
}
