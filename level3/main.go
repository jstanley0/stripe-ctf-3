package main

import (
  "bytes"
  "encoding/json"
  "fmt"
  "index/suffixarray"
  "io/ioutil"
  "log"
  "net/http"
  "os"
  "path/filepath"
  "sort"
  "strconv"
  "strings"
)

type Status struct {
  Success bool `json:"success"`
}

type Searcher struct {
  id        int
  isIndexed bool
  base_path string
  files     map[string]*suffixarray.Index
}

func WriteStatus(w http.ResponseWriter, status bool) error {
  status_msg := Status{
    Success: status,
  }
  b, err := json.Marshal(status_msg)
  w.Write(b)
  return err
}

func host_path(id int) string {
  return fmt.Sprintf("http://localhost:%d", 9090+id)
}

func (s *Searcher) HealthCheck(w http.ResponseWriter, req *http.Request) {
  if s.id == 0 {
    if !s.collectSuccess(w, "healthcheck") {
      WriteStatus(w, false)
      return
    }
  }
  WriteStatus(w, true)
}

func (s *Searcher) Index(w http.ResponseWriter, req *http.Request) {
  qs := req.URL.Query()
  path := qs["path"][0]
  if s.id == 0 {
    // Send job to other servers
    for id := 1; id <= 3; id++ {
      go func(id int) {
        resp, err := http.Get(host_path(id) + "/index?path=" + path)
        if err != nil {
          log.Printf("[master] index failed: %d (%s)", id, err.Error())
        } else {
          resp.Body.Close()
        }
      }(id)
    }
  }
  if stat, err := os.Stat(path); err == nil && stat.IsDir() {
    if strings.HasSuffix(path, "/") {
      s.base_path = path
    } else {
      s.base_path = path + "/"
    }
    go s.doIndex()
    WriteStatus(w, true)
  } else {
    WriteStatus(w, false)
  }
}

func (s *Searcher) collectSuccess(w http.ResponseWriter, name string) bool {
  c := make(chan bool, 4)
  for id := 1; id <= 3; id++ {
    go func(id int) {
      resp, err := http.Get(host_path(id) + "/" + name)
      if err != nil {
        log.Printf("[master] %s failed: %d (%s)", name, id, err.Error())
        c <- false
        return
      }
      body, err := ioutil.ReadAll(resp.Body)
      resp.Body.Close()
      log.Printf("[master] %d %s: %s", id, name, string(body))
      if !bytes.Contains(body, []byte("\"success\":true")) {
        c <- false
        return
      }
      c <- true
    }(id)
  }
  x, y, z := <-c, <-c, <-c
  return x && y && z
}

func (s *Searcher) IsIndexed(w http.ResponseWriter, req *http.Request) {
  // if master isn't done, no sense querying slaves
  if !s.isIndexed {
    WriteStatus(w, false)
    return
  }
  // query slaves
  if s.id == 0 {
    if !s.collectSuccess(w, "isIndexed") {
      WriteStatus(w, false)
      return
    }
    log.Printf("[master] All indexed!")
  }
  WriteStatus(w, true)
}

func (s *Searcher) Query(w http.ResponseWriter, req *http.Request) {
  qs := req.URL.Query()
  query := []byte(qs["q"][0])

  //log.Printf("[%d] Received query: %s", s.id, string(query))

  channel := make(chan []byte, 5)
  if s.id == 0 {
    // send this off to the subprocs asynchronously
    for id := 1; id <= 3; id++ {
      go func(id int) {
        resp, err := http.Get(host_path(id) + "/?q=" + string(query))
        if err != nil {
          log.Printf("[master] onoz slave query failed %d (%s)", id, err.Error())
        }
        b, err := ioutil.ReadAll(resp.Body)
        if err != nil {
          log.Printf("[master] onoz! slave query failed %d (%s)", id, err.Error())
        }
        channel <- b
        resp.Body.Close()
      }(id)
    }
  }

  var results []string
  for filename, index := range s.files {
    hits := index.Lookup(query, -1)
    if len(hits) > 0 {
      line := 1
      lastline := 0
      data := index.Bytes()
      bx := 0
      sort.Ints(hits)
      for _, pos := range hits {
        for bx < pos {
          ll := bytes.IndexByte(data[bx:pos], '\n')
          if ll >= 0 {
            bx += ll + 1
            line += 1
          } else {
            bx = pos
            break
          }
        }
        //log.Printf("hit of %s at offset %d (line %d)", string(query), pos, line)
        if line != lastline {
          result := fmt.Sprintf("\"%s:%d\"", filename, line)
          results = append(results, result)
        }
        lastline = line
      }
    }
  }

  if s.id == 0 {
    //log.Printf("[%d] found %d hits", s.id, len(results))
    for i := 1; i <= 3; i++ {
      b := <-channel
      if len(b) > 0 {
        results = append(results, strings.Split(string(b), "\n")...)
      }
    }
    w.Write([]byte(fmt.Sprintf("{\"success\":true,\"results\":[%s]}", strings.Join(results, ","))))

  } else {
    // dump raw results, one per line
    w.Write([]byte(strings.Join(results, "\n")))
    //log.Printf("[%d] forwarding %d hits", s.id, len(results))
  }
}

func (s *Searcher) indexFile(path string, info os.FileInfo, err error) error {
  // only index 1/4 of the files per server
  if int(path[len(path)-1])%4 != s.id {
    return nil
  }
  if info.Mode().IsRegular() && info.Size() < (1<<20) {
    name := strings.TrimPrefix(path, s.base_path)
    data, _ := ioutil.ReadFile(path)
    s.files[name] = suffixarray.New(data)
  }
  return nil
}

func (s *Searcher) doIndex() {
  err := filepath.Walk(s.base_path, s.indexFile)
  s.isIndexed = (err == nil)
}

func main() {
  id := 0
  args := os.Args
  n := len(args)
  for i := 1; i < n; i++ {
    if args[i] == "--id" && i < n-1 {
      i++
      id, _ = strconv.Atoi(args[i])
    }
  }

  log.Printf("I am node %d", id)
  port := 9090 + id

  s := &Searcher{
    id:        id,
    isIndexed: false,
    files:     make(map[string]*suffixarray.Index),
  }

  http.HandleFunc("/healthcheck", s.HealthCheck)
  http.HandleFunc("/index", s.Index)
  http.HandleFunc("/isIndexed", s.IsIndexed)
  http.HandleFunc("/", s.Query)
  err := http.ListenAndServe(fmt.Sprintf("localhost:%d", port), nil)
  if err != nil {
    log.Fatal("ListenAndServe: ", err)
  }
}
