#!/bin/bash

set -o xtrace
set -o errexit

cd "$(dirname $0)"

# https://stackoverflow.com/questions/3174883/how-to-remove-last-directory-from-a-path-with-sed
export LOGDIR="${PWD%/*}/bin/log"

export PATH=$PATH:/usr/local/go/bin

## Important: if we wanted to build do this
## pushd ../cmd/smokey && go build && popd

./smokey -debug
