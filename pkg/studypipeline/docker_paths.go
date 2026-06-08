package studypipeline

import (
	"os"
	"path"
	"strings"
)

func dockerHostMountPath(containerPath string) string {
	hostDataDir := strings.TrimSpace(os.Getenv("MOODLE_DOCKER_HOST_DATA_DIR"))
	if hostDataDir == "" {
		return containerPath
	}
	containerDataDir := strings.TrimSpace(os.Getenv("MOODLE_DOCKER_CONTAINER_DATA_DIR"))
	if containerDataDir == "" {
		containerDataDir = strings.TrimSpace(os.Getenv("MOODLE_HOME"))
	}
	if containerDataDir == "" {
		containerDataDir = "/data"
	}

	cleanContainerPath := cleanDockerPath(containerPath)
	cleanContainerDataDir := cleanDockerPath(containerDataDir)
	if cleanContainerPath == cleanContainerDataDir {
		return cleanDockerPath(hostDataDir)
	}
	prefix := strings.TrimRight(cleanContainerDataDir, "/") + "/"
	if strings.HasPrefix(cleanContainerPath, prefix) {
		return path.Join(cleanDockerPath(hostDataDir), strings.TrimPrefix(cleanContainerPath, prefix))
	}
	return containerPath
}

func cleanDockerPath(value string) string {
	cleaned := path.Clean(strings.ReplaceAll(value, "\\", "/"))
	if len(cleaned) >= 3 && cleaned[1] == ':' && cleaned[2] == '/' {
		cleaned = cleaned[2:]
	}
	if cleaned == "" {
		return "."
	}
	return cleaned
}
