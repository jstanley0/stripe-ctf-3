
#!/bin/sh
killall -9 main
cd "$(dirname "$0")"
cwd=$(pwd -P)
if [ -z "${CTF_BUILD_ENV}" ]; then
    export GOPATH="$cwd/.build"
else
    export GOPATH="$cwd/.build:$GOPATH"
fi
go get -d
go build ./main.go
