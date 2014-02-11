# Stripe CTF 3 submissions

## Level0

### Score: 1651; leaderboard position: 64 / 3700

I got a passing score of 85 immediately, just by putting the dictionary words in a Hash instead of an Array.  A few minor tweaks to the Ruby code brought this up to 143, at which point I decided to rewrite this in C++.

My principal insight was that the dictionary was already sorted, so spending the time to build a random-access data structure such as a Hash out of a quarter million words, only to do a few hundred lookups, is probably not worthwhile.  Instead, I read the dictionary straight into memory and performed lookups via binary search.

Most of the speed I gained on this came from micro-optimizations to minimize the number of I/Os and freestore allocations.  I read the dictionary into memory in one block, split the words in place, and build an array of pointers to the beginning of each word.  This array excludes all words containing upper-case letters, because they'll never match the downcased document words queried by the original code.

Then I process the document in 64K chunks, avoiding an allocation for each line.  I use std::binary_search to look up each word (using an inlined string comparator instead of strcmp yielded significant savings), and I move the last partial word to the beginning of the buffer before reading the next chunk.  I also buffer output to minimize the number of system calls on output.

In terms of big-O runtime complexity, the broken reference solution was O(NM), where N is the size of the dictionary and M is the size of the document.  Building a hash table could make it O(N + M), which sounds better than my O(MlogN) solution, and might indeed be faster than my solution for sufficiently large M.  But for this specific problem, minimizing I/O and heap overhead was a big win.

My solution ended up 32 times faster than the passing threshold, but still 2.5x slower than the winners, who I suspect pre-built or cached O(1) lookup structures (in addition to clever optimizations).  I would have loved to do something like this too, but it didn't occur to me until late in the contest and I was busy enough with the later levels.

## Level1

### Score: 1000; leaderboard position: 38/1400

I kept the bulk of the provided shell script to manage the git repository, but replaced the loop with a C++ program that accepted the commit object on stdin, searched for a nonce that would make the hash fit the difficulty, and wrote the updated commit to a file. The optimization that probably made my program competitive (at least until the GPU miners stepped in) was to hash the commit once and then pass a copy of the in-progress SHA context to each worker thread, which only had to hash nonces over and over.  Another critical strategy was to choose an appropriate timeout.  If no match was found in 5-30 seconds (a parameter I tweaked based on the current block rate), I would give up and re-update the repository, because somebody probably found a coin during that time.

Scoring on this level was strange.  If you mined a single coin, your score jumped from 50 to somewhere in the neighborhood of 950.  Then, for every other round where you mined a coin, your position is ranked against other successful miners, and if you're near the bottom, you lose points.  This means you are better off mining 0 coins than mining 1 or 2, because your score will not change if you had no success at all during a round.  (It also means you're best off not joining in the middle of a round).

I fired up my miner on three machines early on Saturday morning and won a round with 12 gitcoins mined out of 43 total.  A GPU miner stepped in a bit later and pulled in ~200.

## Level2

### Score: 172; leaderboard position: 99/1100

I don't have much to say about this one, other than various solutions I tried were pretty much indistinguishable score-wise, and my top-100 place was earned only by a lucky run.

## Level3

### Score: 12124; leaderboard position: 24/461

I originally scored 475 on this level by making the Scala code shell out to fgrep instead of performing its inefficient search.  The build time was so painful (it sometimes took as long as 10 minutes) that I couldn't bring myself to try and implement a "real" solution.  But after completing level4, having learned a little Go, I decided to re-implement this one in Go, which seems like it is made for this kind of problem (probably because it is). The channels and goroutines make asynchronously farming out a request and collecting the responses almost magically easy.

I started out dividing the files in thirds (one to each slave node), but then decided to make it fourths (no reason the master couldn't index too).  I used the index/suffixarray package to index each file separately; then, for each file with hits, I counted preceding newlines to figure out line numbers (since Go's text search library returns byte offsets).  The logic was slightly tricky and the online test cases exposed bugs that local ones did not (such as multiple hits per line), making for some frantic last-minute debugging.  But I got the solution hacked together and passing literally at the last minute... or what _would_ have been the last minute had the contest not been extended. :P

## Level4

### Score: 5341; leaderboard position: 15/216

I had never used Go before this problem, but I can fairly say that it felt quite refreshing after dealing with Scala.  Go has some really nice features (and also a serious case of OCD about things like unused imports and variables).  I really like the fast compile times, though.  (Did I mention the comparison with Scala?)

I felt quite lost at the beginning of this one, until I found goraft and raftd, which looked suspiciously familiar to the test program.  It didn't take too long to hack goraft into the project... but it took forever to get the UNIX sockets working with the test harness.  Ugh.  That was the second-biggest headache about the project.

The biggest headache was, of course, forwarding queries from followers to the leader. My early solutions would lose a response from the leader, and the test harness would stay waiting for a query with sequence number 9 while denying any points for the next 600 queriers that went through successfully.  I tried variations of hashing the queries and caching responses and getting results from the replication log, but couldn't quite get it right.  Then I thought, why not just do a 302 redirect?

A friend had tried this already, without success, because your leader's connection string (./nodeX.sock) isn't the same one the test harness uses (something like /tmp/(long hex number)/nodeX/nodeX.sock).  However, the harness gives you that information in the HTTP Host header, so a node could forward to another one just by replacing its own node ID in the Host header with the leader's.

It didn't occur to me during the contest that this actually renders the whole failover moot, since hosts never actually went down--they just became unreachable temporarily, and the 302 totally circumvents that.  So after the contest I went back and got my query forwarding working, because I felt bad about it.

Other optimizations I made included using an in-process, in-memory sqlite database instead of shelling out to sqlite3 (requiring some finagling to get the error messages matching) and compressing the information in the replication log.

Still, the only way to get a high score on this one was to push repeatedly and hope for a good test case.  My implementation could get a top-20 score one run and then fail the next.
