#include <pthread.h>
#include <stdio.h>
#include <string.h>
#include <openssl/sha.h>
#include <string>
#include <stdlib.h>
#include <unistd.h>
#include <sys/time.h>

using namespace std;

#define MAX_COMMIT 4096
#define MAX_THREADS 16

const unsigned char *g_targetdigest;
const SHA_CTX *g_partialhash;
pthread_mutex_t g_mutex = PTHREAD_MUTEX_INITIALIZER;
volatile bool g_stop = false;
volatile bool g_solved = false;
char g_solution_buf[11];
double g_timeout;

struct worker_info
{
  pthread_t thread;
  char tag;
};

void log(const char *tag, char thread_tag, const unsigned char *data, size_t size)
{
  fprintf(stderr, "(%c) %s ", thread_tag, tag);
  for(size_t i = 0; i < size; ++i)
  {
    fprintf(stderr, "%02x", data[i]);
  }
  fputc('\n', stderr);
}

double get_timestamp()
{
  timeval ts;
  gettimeofday(&ts, NULL);
  return ts.tv_sec + (double)ts.tv_usec / 1.0e6;
}

void *timeout_proc(void *params)
{
  int sleeps = (int)(g_timeout * 10.0);
  if (sleeps < 1) sleeps = 1;
  while(!g_stop && sleeps)
  {
    usleep(100000);
    --sleeps;
  }
  g_stop = true;
  pthread_exit(NULL);
  return NULL;
}

void *find_digest(void *params)
{
  worker_info *info = (worker_info *)params;

  unsigned char digest[20];
  char nbuf[11];
  SHA_CTX ctx2;

  srand((unsigned)time(NULL));
  sprintf(nbuf, "%c%04x%04x\n", info->tag, rand(), rand());

  while(!g_stop)
  {
    int dig = 8;
    while (dig >= 1) {
      if (nbuf[dig] == '~')
        nbuf[dig--] = ' ';
      else break;
    }
    nbuf[dig]++;

    memcpy(&ctx2, g_partialhash, sizeof(SHA_CTX));
    SHA1_Update(&ctx2, nbuf, 10);
    SHA1_Final(digest, &ctx2);
    if (memcmp(digest, g_targetdigest, 20) < 0)
    {
      if (0 == pthread_mutex_trylock(&g_mutex))
      {
        if (!g_solved)
        {
          log("solution", info->tag, digest, 20);
          g_stop = true;
          g_solved = true;
          memcpy(g_solution_buf, nbuf, 11);
        }
        pthread_mutex_unlock(&g_mutex);
      }
      break;
    }
  }

  pthread_exit(NULL);
  return NULL;
}

int main(int argc, char **argv)
{
  if (argc < 4)
  {
    fprintf(stderr, "Usage: miner difficulty threadcount timeout\n");
    return 1;
  }
  int num_threads = atoi(argv[2]);
  if (num_threads < 1) num_threads = 1;
  else if (num_threads > MAX_THREADS) num_threads = MAX_THREADS;
  fprintf(stderr, "(-) using %d threads\n", num_threads);

  g_timeout = atof(argv[3]);

  char commit_buf[MAX_COMMIT];
  size_t sz = fread(commit_buf, 1, MAX_COMMIT, stdin);

  double before = get_timestamp();

  unsigned char target_digest[20];
  memset(target_digest, '\x00', 20);
  string difficulty = argv[1];
  if (difficulty.size() > 40)
    difficulty.resize(40);
  else
    difficulty += string(40 - difficulty.size(), '0');
  for(size_t s = 0; s < 20; s++)
  {
    string xd = difficulty.substr(s * 2, 2);
    target_digest[s] = (unsigned char)stoul(xd, NULL, 16);
  }
  log("target  ", '-', target_digest, 20);
  g_targetdigest = target_digest;

  char size_line[100];
  snprintf(size_line, 100, "commit %lu", sz + 10);

  SHA_CTX ctx;
  SHA1_Init(&ctx);
  SHA1_Update(&ctx, size_line, strlen(size_line) + 1);
  SHA1_Update(&ctx, commit_buf, sz);
  g_partialhash = &ctx;

  char tag = 'A';
  worker_info workers[MAX_THREADS];
  for(int i = 0; i < num_threads; ++i)
  {
    workers[i].tag = tag++;
    pthread_create(&workers[i].thread, NULL, find_digest, &workers[i]);
  }

  pthread_t timeout_thread;
  pthread_create(&timeout_thread, NULL, timeout_proc, NULL);
  for(int i = 0; i < num_threads; ++i)
  {
    pthread_join(workers[i].thread, NULL);
  }
  pthread_join(timeout_thread, NULL);

  double after = get_timestamp();
  fprintf(stderr, "(-) elapsed time: %f s\n", after - before);

  if (g_solved)
  {
    fwrite(commit_buf, 1, sz, stdout);
    fwrite(g_solution_buf, 1, 10, stdout);
    return 0;
  }

  fprintf(stderr, "(-) no solution found\n");
  return 1;
}

