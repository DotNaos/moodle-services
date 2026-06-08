# Study Codex refinement

The study pipeline stores extracted Moodle content and Codex-improved content separately.
Machine output remains under:

- `/srv/moodle-study/courses/{courseID}/extracted`

Codex output is written to:

- `/srv/moodle-study/courses/{courseID}/improved/script`
- `/srv/moodle-study/courses/{courseID}/improved/tasks`

The API starts Codex through Docker so each Moodle user gets a separate Codex home
directory:

- `/srv/moodle-study/codex-users/{clerkUserID}`

Build the runner image on the server:

```sh
docker build -t moodle-study-codex-runner:local docker/codex-runner
```

Set these environment variables for the `moodle-services` API:

```sh
MOODLE_STUDY_ARTIFACT_ROOT=/srv/moodle-study
MOODLE_STUDY_CODEX_DOCKER_IMAGE=moodle-study-codex-runner:local
MOODLE_STUDY_CODEX_MODEL_CANDIDATES='gpt5.3 Codex Spark,gpt5.5'
```

The API container needs access to `/var/run/docker.sock`. The default
`docker-compose.yml` mounts it.

`MOODLE_STUDY_CODEX_MODEL_CANDIDATES` is ordered. The API tries the first model
and falls back to the next one if Codex cannot return content. Keep model names
in environment configuration rather than in source code.

Optional override:

```sh
MOODLE_STUDY_CODEX_CONTAINER_COMMAND='codex exec --skip-git-repo-check --sandbox read-only --model "$CODEX_MODEL" -'
```

