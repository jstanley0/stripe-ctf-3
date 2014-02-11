package sql

import (
	"sync"
	"code.google.com/p/go-sqlite/go1/sqlite3"
  "strings"
	"fmt"
	"io"
)

type SQL struct {
	sequenceNumber int
	mutex          sync.Mutex
	conn *sqlite3.Conn

}

type Output struct {
	Stdout         []byte
	Stderr         []byte
	SequenceNumber int
}

func NewSQL(path string) *SQL {
  conn, _ := sqlite3.Open(":memory:")
	sql := &SQL{
		conn: conn,
	}
	return sql
}

func finagleError(line int, error string) string {
  return fmt.Sprintf("Error: near line %d: ", line) + strings.TrimPrefix(error, "sqlite3: ") + "\n"
}

func filterErrors(output string) string {
  if strings.HasPrefix(output, "Error: ") {
    codes := make(map[string]int)
    errors := strings.Split(output, "\n")
    filtered_errors := make([]string, len(errors))
    count := 0
    for _, error := range errors {
      isep := strings.LastIndex(error, " ")
      if isep < 0 {
        continue
      }
      new_error := error[:isep]
      code := error[isep+1:]
      if prev_err_pos, ok := codes[code]; ok {
        filtered_errors[prev_err_pos] = new_error
      } else {
        filtered_errors[count] = new_error
        codes[code] = count
        count++
      }
    }
    var result string
    for _, fe := range filtered_errors {
      if len(fe) > 0 {
        result = result + fe + "\n"
      }
    }
    return result
  } else {
    return output
  }
}

func (sql *SQL) doSql(lineno int, command string) string {
  var output string
  s, err := sql.conn.Prepare(command)
  if err != nil {
     output = finagleError(lineno, err.Error())
  } else {
     err = s.Query()
     if err != nil && err != io.EOF {
        output = finagleError(lineno, err.Error())
     } else {
        if err != io.EOF {
          for {
            var s1, s2 string
            var n1, n2 int
            s.Scan(&s1, &n1, &n2, &s2)
            line := fmt.Sprintf("%s|%d|%d|%s\n", s1, n1, n2, s2)
            output = output + line
            if (s.Next() != nil) {
              break
            }
          }
       }
       s.Close()
     }
  }
  return output
}

func (sql *SQL) Execute(tag string, command string) (*Output, error) {
	defer func() { sql.sequenceNumber += 1 }()

  queries := strings.Split(command, ";")
  var output string
  for line, query := range queries {
    if len(query) == 0 {
      continue
    }
    output = output + sql.doSql(line + 1, query)
  }
  output = filterErrors(output)

  var blank []byte
	outputv := &Output{
		Stdout:         []byte(output),
		Stderr:         blank,
		SequenceNumber: sql.sequenceNumber,
	}

	return outputv, nil
}
