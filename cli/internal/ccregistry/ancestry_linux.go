//go:build linux

package ccregistry

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
)

// parentOf reads the ppid field of /proc/<pid>/stat. The comm field before it is
// parenthesised and may itself hold spaces or parentheses, so the fields are taken
// after the LAST ')'.
func parentOf(pid int) (int, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, err
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0, fmt.Errorf("/proc/%d/stat: no comm field", pid)
	}
	f := bytes.Fields(b[i+1:]) // state ppid ...
	if len(f) < 2 {
		return 0, fmt.Errorf("/proc/%d/stat: no ppid field", pid)
	}
	return strconv.Atoi(string(f[1]))
}
