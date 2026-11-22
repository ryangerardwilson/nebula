package cli

import (
	"strings"
	"testing"
)

func TestLoadExportsShellNames(t *testing.T) {
	fake := mapOp()
	runner, _, stderr, clipboard := newRunner(t, fake)
	stdout, rc := runPiped(t, runner, "wiom", "--load")
	if rc != 0 {
		t.Fatalf("rc = %d\nstderr: %s", rc, stderr.String())
	}
	want := "export WIOM_SNOWFLAKE_ACCOUNT='acct-secret'\nexport WIOM_SNOWFLAKE_USER='user-secret'\nexport hissa='hissa-secret'\nexport keka='keka-secret'\n"
	if stdout != want {
		t.Fatalf("stdout = %q", stdout)
	}
	if stderr.String() != "Skipped 1 items whose names are not shell variables.\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if clipboard.copied {
		t.Fatal("shell load copied to the clipboard")
	}
	if _, ok := fake.call("read --no-newline op://vault-id/odd/password"); ok {
		t.Fatal("skipped item was read")
	}
	assertNoSecret(t, "", stderr.String(), "acct-secret", "user-secret", "hissa-secret", "keka-secret", "odd' secret")
}

func TestLoadRefusesATerminal(t *testing.T) {
	fake := mapOp()
	runner, stdout, stderr, clipboard := newRunner(t, fake)
	rc := runner.Run([]string{"wiom", "--load"})
	if rc != 1 || stdout.Len() != 0 {
		t.Fatalf("rc = %d stdout = %q", rc, stdout.String())
	}
	if stderr.String() != "Load prints export lines for the shell. Run: eval \"$(nebula crypt load wiom)\"\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if clipboard.copied || len(fake.calls) != 0 {
		t.Fatal("terminal load read or copied a secret")
	}
}

func TestLoadEmptyVaultFailsClosed(t *testing.T) {
	runner, _, stderr, clipboard := newRunner(t, &fakeOp{})
	stdout, rc := runPiped(t, runner, "Empty", "--load")
	if rc != 1 || stdout != "" || stderr.String() != "No items in vault Empty.\n" {
		t.Fatalf("rc = %d stdout = %q stderr = %q", rc, stdout, stderr.String())
	}
	if clipboard.copied {
		t.Fatal("empty vault copied a secret")
	}
}

func TestLoadSkipsItemsThatAreNotShellVariables(t *testing.T) {
	fake := &fakeOp{list: `[
	  {"id":"mail","title":"ada@example.com","category":"LOGIN","vault":{"id":"vault-id"}},
	  {"id":"note","title":"NOTES","category":"SECURE_NOTE","vault":{"id":"vault-id"}}
	]`}
	runner, _, stderr, clipboard := newRunner(t, fake)
	stdout, rc := runPiped(t, runner, "wiom", "--load")
	if rc != 1 || stdout != "" {
		t.Fatalf("rc = %d stdout = %q", rc, stdout)
	}
	want := "Skipped 2 items whose names are not shell variables.\nNo items in vault wiom have a shell variable name.\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if clipboard.copied {
		t.Fatal("skipped vault copied a secret")
	}
	for _, call := range fake.calls {
		if strings.HasPrefix(call.args, "read ") {
			t.Fatalf("read a skipped item: %s", call.args)
		}
	}
}

func TestLoadExportsARepeatedTitleOnce(t *testing.T) {
	fake := &fakeOp{
		list: `[
		  {"id":"a","title":"SAME_KEY","category":"API_CREDENTIAL","vault":{"id":"vault-id"}},
		  {"id":"b","title":"SAME_KEY","category":"API_CREDENTIAL","vault":{"id":"vault-id"}}
		]`,
		reads: map[string]string{"/a/credential": "first-secret", "/b/credential": "second-secret"},
	}
	runner, _, stderr, clipboard := newRunner(t, fake)
	stdout, rc := runPiped(t, runner, "wiom", "--load")
	if rc != 0 || stdout != "export SAME_KEY='first-secret'\n" {
		t.Fatalf("rc = %d stdout = %q stderr = %q", rc, stdout, stderr.String())
	}
	if _, ok := fake.call("read --no-newline op://vault-id/b/credential"); ok {
		t.Fatal("duplicate title was read")
	}
	if clipboard.copied || strings.Contains(stderr.String(), "first-secret") {
		t.Fatal("secret leaked")
	}
}

func TestLoadQuotesASecret(t *testing.T) {
	fake := &fakeOp{
		list:  `[{"id":"q","title":"QUOTED","category":"API_CREDENTIAL","vault":{"id":"vault-id"}}]`,
		reads: map[string]string{"/q/credential": "p'q"},
	}
	runner, _, stderr, clipboard := newRunner(t, fake)
	stdout, rc := runPiped(t, runner, "wiom", "--load")
	if rc != 0 || stdout != "export QUOTED='p'\\''q'\n" {
		t.Fatalf("rc = %d stdout = %q stderr = %q", rc, stdout, stderr.String())
	}
	if clipboard.copied {
		t.Fatal("quoted load copied to the clipboard")
	}
}

func TestLoadFailsClosedWhenASecretIsMissing(t *testing.T) {
	fake := &fakeOp{list: `[{"id":"bad","title":"SHELL_NAME","category":"API_CREDENTIAL","vault":{"id":"vault-id"}}]`}
	runner, _, stderr, clipboard := newRunner(t, fake)
	stdout, rc := runPiped(t, runner, "wiom", "--load")
	if rc != 1 || stdout != "" {
		t.Fatalf("rc = %d stdout = %q", rc, stdout)
	}
	if !strings.Contains(stderr.String(), "No secret available for SHELL_NAME.") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if clipboard.copied {
		t.Fatal("failed load copied a secret")
	}
}

func TestLoadRejectsExtraArguments(t *testing.T) {
	fake := mapOp()
	runner, stdout, stderr, clipboard := newRunner(t, fake)
	rc := runner.Run([]string{"wiom", "--load", "extra"})
	if rc != 1 || stdout.Len() != 0 || stderr.String() != "Use: nebula crypt load <vault>\n" {
		t.Fatalf("rc = %d stdout = %q stderr = %q", rc, stdout.String(), stderr.String())
	}
	if clipboard.copied || len(fake.calls) != 0 {
		t.Fatal("bad load called op")
	}
}
