#!/bin/bash

set -eu

if [ "$#" != 4 ]; then
    echo >&2 "Usage: $0 <clone_url> <public_username> <local_dir> <timeout>

Arguments:

<clone_url> is the string you'd pass to git clone (i.e.
  something of the form username@hostname:path)

<public_username> is the public username provided to you in
  the CTF web interface.

<local_dir> is where to put the blockchain

<timeout> in seconds before giving up and re-updating the blockchain
  "
    exit 1
fi

export clone_spec=$1
export public_username=$2
export local_path=$3
export timeout=$4

prepare_index() {
    perl -i -pe 's/($ENV{public_username}: )(\d+)/$1 . ($2+1)/e' LEDGER.txt
    grep -q "$public_username" LEDGER.txt || echo "$public_username: 1" >> LEDGER.txt

    git add LEDGER.txt
}

solve() {
    # Brute force until you find something that's lexicographically
    # small than $difficulty.
    difficulty=$(cat difficulty.txt)

    # Create a Git tree object reflecting our current working
    # directory
    tree=$(git write-tree)
    parent=$(git rev-parse HEAD)
    timestamp=$(date +%s)

	body="tree $tree
parent $parent
author Jeremy Stanley <jstanley0@example.com> $timestamp +0000
committer Jeremy Stanley <jstanley0@example.com> $timestamp +0000

I can has gitcoin?
"
  rm -f newbody
  echo "$body" | ./miner $difficulty 8 $timeout > newbody

	sha1=$(git hash-object -t commit -w newbody)
	git reset --hard "$sha1" > /dev/null
}

reset() {
    git fetch origin master >/dev/null 2>/dev/null
    git reset --hard origin/master >/dev/null
    git pull >/dev/null
}

# Set up repo
if [ -d "$local_path" ]; then
    echo "Using existing repository at $local_path"
    cd "$local_path"
    git reset --hard origin/HEAD >/dev/null
    git pull >/dev/null
    cat LEDGER.txt
else
    echo "Cloning repository to $local_path"
    git clone "$clone_spec" "$local_path"
    cd "$local_path"
    cat LEDGER.txt
fi

prepare_index
solve
    if git push origin master; then
	echo "Success :)"

    else
	echo "Fail :("
    fi

