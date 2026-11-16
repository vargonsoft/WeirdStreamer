#!/bin/sh
# Prints the development version of a build, named as release.sh names a release: the app's version
# and the commit count, with -dev (0.0.1.455-dev). Server and cli builds that aren't release.sh's use it.
cd "$(dirname "$0")/../.." || exit 1
base=$(sed -n 's/.*versionName = "\([0-9.]*\)\.\$buildNumber".*/\1/p' Android/app/build.gradle.kts)
count=$(git rev-list --count HEAD 2>/dev/null || echo 0)
echo "${base:-0.0.1}.$count-dev"
