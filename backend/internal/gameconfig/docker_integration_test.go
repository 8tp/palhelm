package gameconfig

import (
	"os"
	"strings"
	"testing"
)

// TestDocumentedDockerMountTopology is run by the release validation in a disposable
// container as UID 1000 with both a single-file bind and the documented directory bind.
func TestDocumentedDockerMountTopology(t *testing.T) {
	if os.Getenv("PALHELM_DOCKER_TOPOLOGY_TEST") != "1" {
		t.Skip("set PALHELM_DOCKER_TOPOLOGY_TEST=1 inside the disposable topology container")
	}
	single := (&Editor{ComposeFile: "/single/docker-compose.yml", Service: "palworld"}).Probe()
	if single.Available || !strings.Contains(single.Reason, "single file") {
		t.Fatalf("single-file bind capability = %#v, want explicit refusal", single)
	}
	directory := (&Editor{ComposeFile: "/compose/docker-compose.yml", Service: "palworld"}).Probe()
	if !directory.Available {
		t.Fatalf("documented writable directory bind was refused: %#v", directory)
	}
}
