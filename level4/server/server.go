package server

import (
  "bytes"
  "encoding/json"
	"github.com/gorilla/mux"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"stripe-ctf.com/sqlcluster/log"
	"stripe-ctf.com/sqlcluster/sql"
	"stripe-ctf.com/sqlcluster/transport"
	"stripe-ctf.com/sqlcluster/util"
	"github.com/goraft/raft"
	"strings"
	"time"
	"encoding/base64"
	"compress/zlib"
)

type Server struct {
	name       string
	path       string
	listen     string
	router     *mux.Router
	raftServer raft.Server
	httpServer *http.Server
	sql        *sql.SQL
	client     *transport.Client
	connection_string string
}

// Creates a new server.
func New(path, listen string) (*Server, error) {
	cs, err := transport.Encode(listen)
	if err != nil {
		return nil, err
	}
  log.Printf("My connection string is %s", cs)

	sqlPath := filepath.Join(path, "storage.sql")
	util.EnsureAbsent(sqlPath)

	s := &Server{
		path:    path,
		name:    strings.Replace(listen, "/", "-", -1),
		listen:  listen,
		connection_string: cs,
		sql:     sql.NewSQL(sqlPath),
		router:  mux.NewRouter(),
		client:  transport.NewClient(),
	}

	return s, nil
}

// Starts the server.
func (s *Server) ListenAndServe(leader string) error {
	var err error

  leader = strings.Replace(leader, "/", "-", -1)
  log.Printf("Initializing Raft Server: %s", s.path)

  // Initialize and start Raft server.
  transporter := raft.NewHTTPTransporter("/raft")
  transporter.Transport.Dial = transport.UnixDialer
  s.raftServer, err = raft.NewServer(s.name, s.path, transporter, nil, s.sql, "")
  if err != nil {
          log.Fatal(err)
  }
  s.raftServer.SetElectionTimeout(200 * time.Millisecond) // default 150ms
  s.raftServer.SetHeartbeatTimeout(80 * time.Millisecond) // default 50ms

  transporter.Install(s.raftServer, s)
  s.raftServer.Start()

  if leader != "" {
          // Join to leader if specified.

          log.Println("Attempting to join leader:", leader)

          if !s.raftServer.IsLogEmpty() {
                  log.Fatal("Cannot join with an existing log")
          }

          for tries := 0; tries < 10; tries += 1 {
            err := s.Join(leader)
            if err == nil {
              break
            }
            log.Printf("Join attempt %d failed; sleeping", tries)
            time.Sleep(200 * time.Millisecond)
          }
          if err != nil {
                  log.Fatal(err)
          }

  } else if s.raftServer.IsLogEmpty() {
          // Initialize the server by joining itself.

          log.Println("Initializing new cluster")

          _, err := s.raftServer.Do(&raft.DefaultJoinCommand{
                  Name:             s.raftServer.Name(),
                  ConnectionString: s.connection_string,
          })
          if err != nil {
                  log.Fatal(err)
          }

  } else {
          log.Println("Recovered from log")
  }

	// Initialize and start HTTP server.
	s.httpServer = &http.Server{
		Handler: s.router,
	}

	s.router.HandleFunc("/sql", s.sqlHandler).Methods("POST")
	s.router.HandleFunc("/join", s.joinHandler).Methods("POST")
	s.router.HandleFunc("/forward", s.forwardHandler).Methods("GET")

	// Start Unix transport
	l, err := transport.Listen(s.listen)
	if err != nil {
		log.Fatal(err)
	}
	return s.httpServer.Serve(l)
}

// This is a hack around Gorilla mux not providing the correct net/http
// HandleFunc() interface.
func (s *Server) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
        s.router.HandleFunc(pattern, handler)
}

// Joins to the leader of an existing cluster.
func (s *Server) Join(leader string) error {
        command := &raft.DefaultJoinCommand{
                Name:             s.raftServer.Name(),
                ConnectionString: s.connection_string,
        }

        var b bytes.Buffer
        json.NewEncoder(&b).Encode(command)
        _, err := s.client.SafePost("http://" + leader, "/join", &b)
        if err != nil {
                return err
        }
        return nil
}

func (s *Server) joinHandler(w http.ResponseWriter, req *http.Request) {
        command := &raft.DefaultJoinCommand{}

        if err := json.NewDecoder(req.Body).Decode(&command); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        if _, err := s.raftServer.Do(command); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
}

// Client operations

// This is the only user-facing function, and accordingly the body is
// a raw string rather than JSON.
func (s *Server) sqlHandler(w http.ResponseWriter, req *http.Request) {
	query, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Printf("Couldn't read body: %s", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Debugf("[%s] Received query: %#v", s.raftServer.State(), string(query))

  // Execute the command against the Raft server.
  leader := s.raftServer.Leader()
  for leader == "" {
  		     time.Sleep(50 * time.Millisecond)
  		     leader = s.raftServer.Leader()
  }
  if s.name != leader {
          my_partial_name := strings.TrimSuffix(strings.TrimPrefix(s.name, ".-"), ".sock")
          leader_partial_name := strings.TrimSuffix(strings.TrimPrefix(leader, ".-"), ".sock")
          redirect_url := "http://" + strings.Replace(req.Host, my_partial_name, leader_partial_name, -1) + "/forward?query=" + encodeQuery(query)
          log.Printf("Redirecting to %s", redirect_url)
          http.Redirect(w, req, redirect_url, 302)
          return
  }

  resp, err := s.raftServer.Do(NewSqlCommand(string(query)))
  if err != nil {
          http.Error(w, err.Error(), http.StatusBadRequest)
          return
  }
  resp_frd := resp.([]byte)
	log.Debugf("[%s] Returning response to %#v: %#v", s.raftServer.State(), string(query), string(resp_frd))
	w.Write(resp_frd)
}

func encodeQuery(query []byte) string {
  var compressed_query bytes.Buffer
  w := zlib.NewWriter(&compressed_query)
  w.Write(query)
  w.Close()
  return base64.URLEncoding.EncodeToString(compressed_query.Bytes())
}

func decodeQuery(encoded_query string) ([]byte, error) {
  compressed_query, err := base64.URLEncoding.DecodeString(encoded_query)
  if err != nil {
    return nil, err
  }
  r, err := zlib.NewReader(bytes.NewBuffer(compressed_query))
  if err != nil {
    return nil, err
  }
  decoded_query, err := ioutil.ReadAll(r)
  if err != nil {
    return nil, err
  }
  r.Close()
  return decoded_query, nil
}

func (s *Server) forwardHandler(w http.ResponseWriter, req *http.Request) {
  query := req.URL.Query()
  encoded_query := query["query"][0]
  decoded_query, err := decodeQuery(encoded_query)
  if err != nil {
             http.Error(w, err.Error(), http.StatusBadRequest)
             return
  }
  resp, err := s.raftServer.Do(NewSqlCommand(string(decoded_query)))
  if err != nil {
          http.Error(w, err.Error(), http.StatusBadRequest)
          return
  }
  resp_frd := resp.([]byte)
	log.Debugf("[%s] Returning response to %#v: %#v", s.raftServer.State(), string(decoded_query), string(resp_frd))
	w.Write(resp_frd)
}
