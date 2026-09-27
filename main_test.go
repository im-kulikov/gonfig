package gonfig

import (
	"flag"
	"os"
	"testing"
)

// TestMain makes every test independent of how it is run. A test that leaves
// Config.Args or Config.Envs nil reads os.Args and os.Environ, and they differ
// between `go test`, an IDE and CI: GoLand passes "-test.run ^TestX$" as two
// arguments, and pflag leaves the pattern as a positional argument.
func TestMain(m *testing.M) {
	flag.Parse() // the -test.* flags, before they are cleared

	os.Args = os.Args[:1]
	os.Clearenv()

	os.Exit(m.Run())
}
