package development

import (
	"strings"
	"testing"
)

func TestContainerArgumentsEnforceStrongIsolation(t *testing.T) {
	sandbox := &ContainerSandbox{
		image: "registry.example/ekbda-go@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		cpus:  "1", memory: "1g", pids: 128, writableTmpSize: "256m", user: "10001:10001",
	}
	arguments := sandbox.arguments(t.TempDir(), Command{ID: "go-test-all", Executable: "go", Arguments: []string{"test", "./..."}})
	joined := strings.Join(arguments, " ")
	for _, required := range []string{
		"--pull=never", "--network=none", "--ipc=none", "--read-only", "--cap-drop=ALL",
		"--security-opt=no-new-privileges", "--pids-limit=128", "--memory=1g", "--cpus=1",
		"--user=10001:10001", "target=/workspace,readonly", "/tmp:rw,noexec,nosuid,nodev,size=256m",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_OPTIONAL_LOCKS=0", "--entrypoint go", sandbox.image, "test ./...",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("container arguments missing %q: %s", required, joined)
		}
	}
	for _, forbidden := range []string{"--privileged", "--network=host", "/var/run/docker.sock"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("container arguments contain forbidden option %q: %s", forbidden, joined)
		}
	}
}

func TestContainerConfigurationRequiresPinnedImage(t *testing.T) {
	_, err := NewContainerSandbox(ContainerConfig{
		Binary: "docker", Image: "golang:latest", CPUs: "1", Memory: "1g", PIDs: 128,
		WritableTmpSize: "256m", User: "10001:10001",
	})
	if err == nil {
		t.Fatal("mutable container image must be rejected before runtime lookup")
	}
}
