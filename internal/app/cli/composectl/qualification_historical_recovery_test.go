package composectl

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This fixture proves restoration of the real predecessor's paired PostgreSQL
// and local object/home state after candidate writes. It is not a receipt for
// the host controller's full operator workflow or a remote object-store profile.
type qualificationHistoricalColdPair struct {
	Directory string
	Digests   map[string][32]byte
}

func verifyQualificationHistoricalLegacyToken(t *testing.T, ctx context.Context, fixture qualificationHistoricalPredecessorFixture, transport *qualificationHistoricalTransport, expected int) {
	t.Helper()
	expires, err := time.Parse(time.RFC3339Nano, fixture.LegacyTokenExpiry)
	require.NoError(t, err)
	require.True(t, time.Now().Before(expires), "an expired token cannot prove migration revocation or restoration")
	client, err := qualificationHistoricalWorkloadHTTPClient(transport)
	require.NoError(t, err)
	endpoint := qualificationHistoricalOrigin + "/api/v1/service-principals"
	require.NoError(t, qualificationHistoricalExpectStatus(ctx, client, http.MethodGet, endpoint, "", http.StatusUnauthorized))
	require.NoError(t, qualificationHistoricalExpectStatus(ctx, client, http.MethodGet, endpoint, fixture.LegacyToken, expected))
}

func captureQualificationHistoricalColdPair(t *testing.T, ctx context.Context, fixture qualificationHistoricalPredecessorFixture) *qualificationHistoricalColdPair {
	t.Helper()
	require.NoError(t, stopQualificationHistoricalContainerGracefully(ctx, fixture.Predecessor))
	require.NoError(t, stopQualificationHistoricalContainerGracefully(ctx, fixture.Topology.Container))
	pair := &qualificationHistoricalColdPair{Directory: privateQualificationTempDir(t), Digests: make(map[string][32]byte)}
	qualificationHistoricalPairCommand(t, ctx, fixture, pair.Directory, true,
		fmt.Sprintf(`tar -cpf /pair/postgres.tar -C /postgres .; tar -cpf /pair/state.tar -C /state .; chmod 600 /pair/*.tar; chown %d:%d /pair/*.tar`, os.Getuid(), os.Getgid()))
	for _, name := range []string{"postgres.tar", "state.tar"} {
		path := filepath.Join(pair.Directory, name)
		contents, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NotEmpty(t, contents)
		pair.Digests[name] = sha256.Sum256(contents)
	}
	restartQualificationHistoricalPostgres(t, ctx, fixture)
	return pair
}

func qualificationHistoricalPairCommand(t *testing.T, ctx context.Context, fixture qualificationHistoricalPredecessorFixture, directory string, readOnly bool, script string) {
	t.Helper()
	require.Equal(t, fixture.ComposeProject+"_qualification-postgres-data", fixture.PostgresVolume)
	require.Equal(t, fixture.ComposeProject+"_leapview-state", fixture.StateVolume)
	for _, container := range []qualificationContainer{fixture.Predecessor, fixture.Topology.Container} {
		state, err := container.Inspect(ctx, "{{.State.Status}}")
		require.NoError(t, err)
		require.Equal(t, "exited", strings.TrimSpace(string(state)), "paired copy requires stopped predecessor and PostgreSQL")
	}
	mode := ""
	if readOnly {
		mode = ",readonly"
	}
	command := exec.CommandContext(ctx, "docker", "run", "--rm", "--network=none", "--user=0",
		"--mount", "type=volume,source="+fixture.PostgresVolume+",target=/postgres"+mode,
		"--mount", "type=volume,source="+fixture.StateVolume+",target=/state"+mode,
		"--mount", "type=bind,source="+directory+",target=/pair", "--entrypoint", "sh",
		qualificationPostgreSQL18Image, "-ec", script)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", redactQualificationBytes(output))
}

func restartQualificationHistoricalPostgres(t *testing.T, ctx context.Context, fixture qualificationHistoricalPredecessorFixture) {
	t.Helper()
	_, err := fixture.Topology.Container.Start(ctx)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err := fixture.Topology.Container.Exec(ctx, nil, "pg_isready", "--host", "localhost", "--username", "leapview_bootstrap", "--dbname", "postgres")
		return err == nil
	}, 2*time.Minute, 250*time.Millisecond, "restored PostgreSQL must accept connections")
}

func restoreQualificationHistoricalColdPair(t *testing.T, ctx context.Context, repoRoot string, fixture qualificationHistoricalPredecessorFixture, candidate qualificationContainer, pair *qualificationHistoricalColdPair) {
	t.Helper()
	require.NoError(t, stopQualificationHistoricalContainerGracefully(ctx, candidate))
	require.NoError(t, stopQualificationHistoricalContainerGracefully(ctx, fixture.Topology.Container))
	for _, name := range []string{"postgres.tar", "state.tar"} {
		contents, err := os.ReadFile(filepath.Join(pair.Directory, name))
		require.NoError(t, err)
		require.Equal(t, pair.Digests[name], sha256.Sum256(contents), "paired recovery archive changed")
	}
	qualificationHistoricalPairCommand(t, ctx, fixture, pair.Directory, false,
		`find /postgres /state -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +; tar -xpf /pair/postgres.tar -C /postgres; tar -xpf /pair/state.tar -C /state`)
	restartQualificationHistoricalPostgres(t, ctx, fixture)
	require.Equal(t, int64(fixture.Identity.Schema), qualificationHistoricalPostgresRevision(t, ctx, fixture.Topology))
	_, err := fixture.Predecessor.Start(ctx)
	require.NoError(t, err)
	endpoint := qualificationHistoricalServerEndpoint(t, ctx, fixture.Predecessor, fixture.Network)
	require.NoError(t, fixture.Proxy.setEndpoint(endpoint))
	readyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	require.NoError(t, waitQualificationHistoricalHealth(readyCtx, endpoint))
	require.Equal(t, fixture.Identity.Revision, qualificationHistoricalContainerRevision(t, ctx, fixture.Predecessor))
	viewer := newQualificationHistoricalBrowser(t, fixture.Proxy.ProxyURL, fixture.Proxy.CACert)
	require.NoError(t, viewer.loginExisting(ctx, fixture.Seed.ViewerEmail, qualificationHistoricalBrowserPassword))
	require.NoError(t, viewer.checkDashboard(ctx, qualificationHistoricalDashboardOverviewURL(fixture.Seed.DashboardID)))
	require.NoError(t, runQualificationHistoricalBrowserValidation(ctx, t, repoRoot, fixture.Seed, fixture.Proxy))
	verifyQualificationHistoricalLegacyToken(t, ctx, fixture, fixture.Proxy, http.StatusOK)
	t.Log("published predecessor restored: schema, exact binary revision, legacy credential, viewer and CFO query proof passed")
}
