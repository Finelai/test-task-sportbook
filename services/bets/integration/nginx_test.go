//go:build integration

package integration

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestNginxResolvesRecreatedBets(t *testing.T) {
	ctx := context.Background()
	nw, err := network.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nw.Remove(ctx) })
	start := func(req testcontainers.ContainerRequest) testcontainers.Container {
		t.Helper()
		c, e := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = c.Terminate(ctx) })
		return c
	}
	backend := func(aliases []string) testcontainers.Container {
		return start(testcontainers.ContainerRequest{Image: "nginx:1.29.4-alpine", Networks: []string{nw.Name}, NetworkAliases: map[string][]string{nw.Name: aliases}, Files: []testcontainers.ContainerFile{{Reader: strings.NewReader("server { listen 8080; listen 8083; location / { return 204; } }"), ContainerFilePath: "/etc/nginx/conf.d/default.conf", FileMode: 0644}}, WaitingFor: wait.ForLog("start worker processes").WithStartupTimeout(time.Minute)})
	}
	first := backend([]string{"bets", "line-bets", "wallet-bets", "contract-check", "swagger-ui"})
	firstIP, err := first.ContainerIP(ctx)
	if err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs("../../../infra/nginx-bets/default.conf")
	if err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	proxy := start(testcontainers.ContainerRequest{Image: "nginx:1.29.4-alpine", Networks: []string{nw.Name}, ExposedPorts: []string{"8080/tcp"}, Files: []testcontainers.ContainerFile{{Reader: strings.NewReader(string(config)), ContainerFilePath: "/etc/nginx/conf.d/default.conf", FileMode: 0644}}, WaitingFor: wait.ForListeningPort("8080/tcp").WithStartupTimeout(time.Minute)})
	host, err := proxy.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := proxy.MappedPort(ctx, "8080/tcp")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + host + ":" + port.Port() + "/api/v1/bets/test"
	client := &http.Client{Timeout: time.Second}
	request := func() int {
		r, e := client.Get(url)
		if e != nil {
			return 0
		}
		_ = r.Body.Close()
		return r.StatusCode
	}
	if status := request(); status != 204 {
		logs, _ := proxy.Logs(ctx)
		var detail []byte
		if logs != nil {
			detail, _ = io.ReadAll(logs)
			_ = logs.Close()
		}
		t.Fatalf("initial status %d; proxy logs %s", status, detail)
	}
	if err = first.Terminate(ctx); err != nil {
		t.Fatal(err)
	}
	occupier := start(testcontainers.ContainerRequest{Image: "alpine:3.23", Networks: []string{nw.Name}, Cmd: []string{"sleep", "60"}})
	occupiedIP, err := occupier.ContainerIP(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second := backend([]string{"bets"})
	secondIP, err := second.ContainerIP(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if secondIP == firstIP || occupiedIP != firstIP {
		t.Fatalf("IP setup: old=%s occupied=%s new=%s", firstIP, occupiedIP, secondIP)
	}
	deadline := time.Now().Add(8 * time.Second)
	status := 0
	for time.Now().Before(deadline) {
		status = request()
		if status == 204 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("proxy status %d after Bets moved from %s to %s", status, firstIP, secondIP)
}
