package server

import (
        "github.com/goraft/raft"
        "stripe-ctf.com/sqlcluster/sql"
        "stripe-ctf.com/sqlcluster/util"
        "fmt"
        "errors"
        "compress/zlib"
        "bytes"
        "io/ioutil"
)

func deflate(query string) []byte {
  var compressed_query bytes.Buffer
  w := zlib.NewWriter(&compressed_query)
  w.Write([]byte(query))
  w.Close()
  return compressed_query.Bytes()
}

func inflate(compressed_query []byte) string {
  r, _ := zlib.NewReader(bytes.NewBuffer(compressed_query))
  b, _ := ioutil.ReadAll(r)
  r.Close()
  return string(b)
}

// This command writes a value to a key.
type SqlCommand struct {
        Query []byte `json:"query"`
}

// Creates a new write command.
func NewSqlCommand(query string) *SqlCommand {
        return &SqlCommand{
                Query:   deflate(query),
        }
}

// The name of the command in the log.
func (c *SqlCommand) CommandName() string {
        return "sql"
}

// Writes a value to a key.
func (c *SqlCommand) Apply(server raft.Server) (interface{}, error) {
  db := server.Context().(*sql.SQL)
  output, err := db.Execute("tehrafts", inflate(c.Query))

	if err != nil {
		var msg string
		if output != nil && len(output.Stderr) > 0 {
			template := `Error executing %#v (%s)

SQLite error: %s`
			msg = fmt.Sprintf(template, c.Query, err.Error(), util.FmtOutput(output.Stderr))
		} else {
			msg = err.Error()
		}

		return nil, errors.New(msg)
	}

	formatted := fmt.Sprintf("SequenceNumber: %d\n%s",
		output.SequenceNumber, output.Stdout)
	return []byte(formatted), nil
}
