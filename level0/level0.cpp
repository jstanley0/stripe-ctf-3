#include <iostream>
#include <stdio.h>
#include <stdlib.h>
#include <string>
#include <vector>
#include <algorithm>
#include <string.h>

using namespace std;

void load_file(const char *filename, string& data)
{
    FILE *f = fopen(filename, "r");
    fseek(f, 0, SEEK_END);
    long int size = ftell(f);
    fseek(f, 0, SEEK_SET);
    data.resize(size);
    fread(&data[0], size, 1, f);
    fclose(f);
}

struct teh_less {
    bool operator()(const char *pl, const char *pr) const {
        while(*pl && (*pl == *pr))
            ++pl, ++pr;
        return *(const unsigned char*)pl-*(const unsigned char*)pr < 0;
    }
};
vector<const char *> g_dict;
string dict_storage;

void load_dict(const char *filename)
{
    load_file(filename, dict_storage);
    g_dict.reserve(234938);
    char *p = &dict_storage[0], *end = p + dict_storage.size();
    while (p < end)
    {
        char *word = p;
        bool lc = true;
        while(*p != '\n') {
            if (lc && *p >= 'A' && *p <= 'Z')
                lc = false;
            ++p;
        }
        *p++ = '\0';
        if (lc)
            g_dict.push_back(word);
    }
}

class Writer
{
  static const int BUFSIZE = 65536;
  char buffer[BUFSIZE];
  char *p, *end;

  void flush()
  {
    fwrite(buffer, 1, p - buffer, stdout);
    p = &buffer[0];
  }

public:
  Writer() : p(&buffer[0]), end(&buffer[BUFSIZE])
  {
    setvbuf(stdout, NULL, _IONBF, 0);
  }

  void putchar(char c)
  {
    if (p == end)
      flush();
    *p++ = c;
  }

  void puts(const char *s, size_t n)
  {
    if (end - p < n)
      flush();
    memcpy(p, s, n);
    p += n;
  }

  ~Writer()
  {
    flush();
  }
} g_Writer;

void dump_word(const char *word, size_t len)
{
    static const size_t MAX=2048;
    static char lword[MAX];
    bool found = false;
    if (len < MAX-1)
    {
      size_t i = 0;
      while(i < len)
      {
        lword[i] = tolower(word[i]);
        i++;
      }
      lword[i] = '\0';
      found = binary_search(g_dict.begin(), g_dict.end(), lword, teh_less());
    }
    if (!found) g_Writer.putchar('<');
    g_Writer.puts(word, len);
    if (!found) g_Writer.putchar('>');
}

int main(int argc, char **argv)
{
    const char *dict_file = (argc >= 2) ? argv[1] : "/usr/share/dict/words";
    load_dict(dict_file);

    static const size_t BUFSIZE = 65536;
    char buf[BUFSIZE];
    char *p = &buf[0], *w = NULL;
    size_t beg = 0;

    for(;;)
    {
      size_t read = fread(&buf[beg], 1, BUFSIZE - beg, stdin);
      char *q = &buf[beg + read];
      if (p == q)
        break;
      while(p < q)
      {
        if (*p == '\n' || *p == ' ')
        {
          if (w && w < p)
          {
            dump_word(w, p - w);
            w = NULL;
          }
          g_Writer.putchar(*p);
        }
        else
        {
          if (!w)
            w = p;
        }
        ++p;
      }

      if (w)
      {
        if (w == &buf[0] || q < &buf[BUFSIZE])
        {
          dump_word(w, p - w);
          w = NULL;
          p = &buf[0];
          beg = 0;
        }
        else
        {
          beg = p - w;
          memmove(&buf[0], w, beg);
          w = &buf[0];
          p = &buf[beg];
        }
      }
      else
      {
        p = &buf[0];
        beg = 0;
      }
    }
}