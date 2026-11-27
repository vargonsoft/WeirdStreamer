#!/usr/bin/env python3
"""Builds the Weather channel's built-in map data (internal/guide/uscities.txt.gz and usstates.txt.gz) from the
Census Bureau's public-domain files:

  2024_Gaz_place_national.txt   the 2024 Gazetteer of places (each place's inner point)
  sub-est2023.csv               the 2023 population estimates of cities and towns
  cb_2023_us_state_20m.shp/.dbf the 2023 cartographic boundaries of the states, 1:20,000,000

  python3 build-weather-maps.py <folder holding them> <internal/guide folder>

uscities: every incorporated place of 25,000 people or more, "name<TAB>ST<TAB>lat<TAB>lon<TAB>population".
usstates: each state's outline, simplified, "ST<TAB>ring;ring..." with each ring "lon,lat lon,lat ...".
"""
import csv
import gzip
import math
import os
import re
import struct
import sys

MIN_POP = 25000
TOLERANCE = 0.03  # degrees: the outlines' simplification
MIN_RING = 0.02   # square degrees: smaller islands are left out

# Names whose Census form isn't how people say them
NAMES = {
    "Nashville-Davidson": "Nashville", "Louisville/Jefferson County": "Louisville", "Lexington-Fayette": "Lexington",
    "Augusta-Richmond County": "Augusta", "Athens-Clarke County": "Athens", "Macon-Bibb County": "Macon",
    "Indianapolis city": "Indianapolis", "Columbus-Muscogee": "Columbus", "Urban Honolulu": "Honolulu",
    "Boise City": "Boise", "San Buenaventura (Ventura)": "Ventura", "Ventura (San Buenaventura)": "Ventura",
}


# Places whose inner point is off from the city people know (San Francisco's is pulled into the Pacific by
# the Farallon Islands): lat, lon
POSITIONS = {("San Francisco", "CA"): (37.7749, -122.4194)}


def clean_name(name):
    name = re.sub(r"\s*\(balance\)$", "", name)
    words = name.split(" ")
    while len(words) > 1 and words[-1].islower():  # "city", "town", "metropolitan government"...
        words.pop()
    name = " ".join(words)
    name = re.sub(r"^.*\((.+)\)$", r"\1", name)  # "El Paso de Robles (Paso Robles)": the name in use
    return NAMES.get(name, name)


def cities(src):
    pop = {}
    with open(os.path.join(src, "sub-est2023.csv"), encoding="latin-1") as f:
        for r in csv.DictReader(f):
            if r["SUMLEV"] == "162":
                pop[r["STATE"] + r["PLACE"]] = int(r["POPESTIMATE2023"])
    out = []
    with open(os.path.join(src, "2024_Gaz_place_national.txt"), encoding="utf-8") as f:
        rows = csv.reader(f, delimiter="\t")
        head = [h.strip() for h in next(rows)]
        for r in rows:
            r = dict(zip(head, (v.strip() for v in r)))
            p = pop.get(r["GEOID"], 0)
            if p >= MIN_POP:
                name = clean_name(r["NAME"])
                lat, lon = POSITIONS.get((name, r["USPS"]), (float(r["INTPTLAT"]), float(r["INTPTLONG"])))
                out.append((name, r["USPS"], lat, lon, p))
    out.sort(key=lambda c: -c[4])
    return out


def dbf_records(path):
    with open(path, "rb") as f:
        data = f.read()
    n, header, size = struct.unpack("<IHH", data[4:12])
    fields, pos = [], 32
    while data[pos] != 0x0D:
        name = data[pos:pos + 11].split(b"\0")[0].decode()
        fields.append((name, data[pos + 16]))
        pos += 32
    recs = []
    for i in range(n):
        rec, at = data[header + i * size:header + (i + 1) * size], 1
        row = {}
        for name, length in fields:
            row[name] = rec[at:at + length].decode("latin-1").strip()
            at += length
        recs.append(row)
    return recs


def shp_polygons(path):
    with open(path, "rb") as f:
        data = f.read()
    pos, shapes = 100, []
    while pos < len(data):
        _, length = struct.unpack(">ii", data[pos:pos + 8])
        content = data[pos + 8:pos + 8 + length * 2]
        pos += 8 + length * 2
        if struct.unpack("<i", content[:4])[0] != 5:
            shapes.append([])
            continue
        nparts, npoints = struct.unpack("<ii", content[36:44])
        parts = list(struct.unpack("<%di" % nparts, content[44:44 + 4 * nparts]))
        at = 44 + 4 * nparts
        pts = [struct.unpack("<dd", content[at + 16 * i:at + 16 * i + 16]) for i in range(npoints)]
        rings = [pts[parts[i]:(parts[i + 1] if i + 1 < nparts else npoints)] for i in range(nparts)]
        shapes.append(rings)
    return shapes


def area(ring):
    return abs(sum(a[0] * b[1] - b[0] * a[1] for a, b in zip(ring, ring[1:] + ring[:1]))) / 2


def simplify(pts, tol):  # Douglas-Peucker
    if len(pts) < 3:
        return pts
    keep = [False] * len(pts)
    keep[0] = keep[-1] = True
    stack = [(0, len(pts) - 1)]
    while stack:
        a, b = stack.pop()
        (x1, y1), (x2, y2) = pts[a], pts[b]
        dx, dy = x2 - x1, y2 - y1
        norm = math.hypot(dx, dy) or 1e-12
        best, at = 0, -1
        for i in range(a + 1, b):
            x, y = pts[i]
            d = abs(dy * (x - x1) - dx * (y - y1)) / norm if (dx or dy) else math.hypot(x - x1, y - y1)
            if d > best:
                best, at = d, i
        if best > tol:
            keep[at] = True
            stack += [(a, at), (at, b)]
    return [p for p, k in zip(pts, keep) if k]


def states(src):
    names = [r["STUSPS"] for r in dbf_records(os.path.join(src, "cb_2023_us_state_20m.dbf"))]
    out = []
    for st, rings in zip(names, shp_polygons(os.path.join(src, "cb_2023_us_state_20m.shp"))):
        kept = []
        for ring in rings:
            if area(ring) < MIN_RING:
                continue
            s = simplify(ring, TOLERANCE)
            if len(s) >= 4:
                kept.append(" ".join("%.2f,%.2f" % p for p in s))
        if kept:
            out.append((st, kept))
    out.sort()
    return out


def main():
    src, dst = sys.argv[1], sys.argv[2]
    cs = cities(src)
    with gzip.open(os.path.join(dst, "uscities.txt.gz"), "wt", encoding="utf-8", compresslevel=9) as f:
        f.write("# US cities of 25,000 people or more (Census Bureau: 2024 Gazetteer, 2023 population estimates; "
                "public domain): name, state, lat, lon, population\n")
        for c in cs:
            f.write("%s\t%s\t%.4f\t%.4f\t%d\n" % c)
    ss = states(src)
    with gzip.open(os.path.join(dst, "usstates.txt.gz"), "wt", encoding="utf-8", compresslevel=9) as f:
        f.write("# US states' outlines, simplified (Census Bureau: 2023 cartographic boundaries, 1:20,000,000; "
                "public domain): state, rings of lon,lat\n")
        for st, rings in ss:
            f.write("%s\t%s\n" % (st, ";".join(rings)))
    print("%d cities, %d states" % (len(cs), len(ss)))


if __name__ == "__main__":
    main()
