package agent

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ReadEnvFile parses KEY=VALUE lines (blank lines and # comments skipped),
// the format of agent.env on Linux and Windows.
func ReadEnvFile(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", n)
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out, sc.Err()
}

// WriteEnvFile writes kv as sorted KEY=VALUE lines.
func WriteEnvFile(w io.Writer, kv map[string]string) error {
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "%s=%s\n", k, kv[k]); err != nil {
			return err
		}
	}
	return nil
}
