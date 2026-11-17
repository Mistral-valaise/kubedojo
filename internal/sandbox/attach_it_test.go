package sandbox

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
)

func shells(t *testing.T, container string) int {
	t.Helper()
	out, _ := exec.Command("docker", "exec", container, "pgrep", "-c", "-u", "candidate", "bash").Output()
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}

// Closing a terminal must end its shell and everything started from it, or
// every closed tab would leave processes behind in the sandbox.
//
// Integration test: needs the terminal container of a running sandbox.
//
//	KUBEDOJO_IT_SESSION=<session id> go test ./internal/sandbox -run AttachClose -v
func TestAttachCloseEndsShell(t *testing.T) {
	id := os.Getenv("KUBEDOJO_IT_SESSION")
	if id == "" {
		t.Skip("set KUBEDOJO_IT_SESSION to the id of a running sandbox")
	}
	k := &Kind{}
	ctx := context.Background()
	before := shells(t, terminalName(id))
	var ttys []TTY
	for i := 0; i < 3; i++ {
		tty, err := k.Attach(ctx, dockerhost.Host{}, id, 100, 30)
		if err != nil {
			t.Fatal(err)
		}
		ttys = append(ttys, tty)
		// Something long-running in the foreground, as a learner would have.
		if _, err := tty.Write([]byte("sleep 4242\n")); err != nil {
			t.Fatal(err)
		}
		go func() { // drain output so the shell is never blocked on a full pipe
			buf := make([]byte, 4096)
			for {
				if _, err := tty.Read(buf); err != nil {
					return
				}
			}
		}()
	}
	time.Sleep(time.Second)
	if got := shells(t, terminalName(id)); got != before+3 {
		t.Fatalf("%d shells while 3 terminals are open, want %d", got, before+3)
	}
	for _, tty := range ttys {
		tty.Close()
	}
	deadline := time.Now().Add(10 * time.Second)
	for shells(t, terminalName(id)) != before && time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
	}
	if got := shells(t, terminalName(id)); got != before {
		t.Errorf("%d shells left after closing all terminals, want %d", got, before)
	}
	out, _ := exec.Command("docker", "exec", terminalName(id), "pgrep", "-fc", "sleep 4242").Output()
	if n := strings.TrimSpace(string(out)); n != "0" {
		t.Errorf("%s foreground processes survived their terminal", n)
	}
}
