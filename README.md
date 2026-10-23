# git-works

Web-Server und Proxy für [opencode](https://opencode.ai) – verwaltet Workspaces und Workloads
in isolierten Containern und stellt sie über GitLab-Tokens authentifiziert bereit.

## Überblick

- **Workspace-Verwaltung**: Projekte aus GitLab klonen, committen und pushen
- **Workload-Proxy**: Opencode-Instanzen (Container) werden pro Workspace gestartet und über den Server proxied
- **Authentifizierung**: GitLab Personal Access Token wird gegen die GitLab-API validiert, Session via HttpOnly-Cookie (`gw_session`)
- **TLS**: Optional über Env-Variablen (siehe unten)

## Build & Start

```bash
go build -o git-works ./cmd/web
./git-works
```

Docker:

```bash
docker build -t git-works .
docker run -p 8080:8080 -v git-works-data:/data git-works
```

Der Opencode-Container wird über `opencode.containerfile` gebaut.

## Environment-Variablen

### Server

| Variable | Default | Beschreibung |
|---|---|---|
| `SERVER_ADDRESS` | `0.0.0.0:8080` | Listen-Adresse des Web-Servers |
| `TLS_CERT_FILE` | – | Pfad zum TLS-Zertifikat (PEM). Wenn gesetzt (zusammen mit `TLS_KEY_FILE`), startet der Server mit HTTPS |
| `TLS_KEY_FILE` | – | Pfad zum privaten TLS-Key (PEM) |
| `DATA_DIR` | `./data` | Basisverzeichnis für Workspace-Daten |
| `GITLAB_BASE_URL` | `https://gitlab.com` | Basis-URL der GitLab-Instanz (API-URL) |
| `GITLAB_ACTIONS_FILE` | `gitlab-http-actions.yml` | Konfiguration der verfügbaren GitLab-HTTP-Aktionen |

### Git

| Variable | Default | Beschreibung |
|---|---|---|
| `SSH_ASKPASS` | `/usr/local/bin/git-askpass` | Pfad zum git-askpass-Helper (liefert den Token für git push) |
| `GIT_TOKEN` | – | GitLab-Token, der vom git-askpass-Helper gelesen wird |

### Workloads / Opencode

| Variable | Default | Beschreibung |
|---|---|---|
| `OPENCODE_REMOTE_HOST` | `127.0.0.1` | Host, unter dem die Opencode-Workloads erreichbar sind |
| `WORKSPACE_START_PORT` | `9000` | Startport für die Portvergabe der Workloads |
| `OPENCODE_TEMPLATE_FILE` | `opencode.json` | Template-Datei für die Opencode-Konfiguration pro Workload |
| `OPENCODE_TARGET_URL` | `http://{{.host}}:{{.port}}/` | URL-Template, unter dem ein Workload erreichbar ist |
| `WORSPACE_LOG` | – | Wenn gesetzt, wird das Workspace-Log in diese Datei geschrieben |

## HTTP-Endpunkte

| Methode & Pfad | Beschreibung |
|---|---|
| `GET /health` | Healthcheck |
| `POST /api/auth/token` | Login mit GitLab-Token, setzt Session-Cookie |
| `GET /api/user` | Aktueller Benutzer |
| `GET /api/gitlab/project` | GitLab-Projekte suchen |
| `GET /api/gitlab/project/{projectid}/branches` | Branches eines Projekts |
| `GET /` | Web-UI (statisch aus `static/git-works`) |
| `GET /static/*` | Statische Assets |

| `PATCH /.git-works/session-mode` | Workspace-Mode der Session aktivieren/deaktivieren |
| `/` (mit aktiver Session + Workload) | Proxy zum aktiven Opencode-Workload |

Workspace-/Projekt-/Workload-API (authentifiziert):

| Methode & Pfad | Beschreibung |
|---|---|
| `GET /api/workspace` | Workspaces auflisten |
| `POST /api/workspace` | Workspace anlegen |
| `GET /api/workspace/{workspaceId}` | Workspace-Detail |
| `DELETE /api/workspace/{workspaceId}` | Workspace löschen |
| `POST /api/workspace/{workspaceId}/init` | Workspace initialisieren |
| `POST /api/workspace/{workspaceId}/project` | Projekt (GitLab-Repo) hinzufügen |
| `DELETE /api/workspace/{workspaceId}/project/{projectId}` | Projekt entfernen |
| `GET /api/workspace/{workspaceId}/project/{projectId}/status` | Projekt-Status (git) |
| `POST /api/workspace/{workspaceId}/project/{projectId}/init` | Projekt klonen/init |
| `POST /api/workspace/{workspaceId}/project/{projectId}/commit` | Änderungen committen |
| `POST /api/workspace/{workspaceId}/project/{projectId}/push` | Branch pushen |
| `GET /api/workspace/{workspaceId}/workload/{workloadId}/status` | Workload-Status |
| `POST /api/workspace/{workspaceId}/workload/{workloadId}/init` | Workload initialisieren |
| `POST /api/workspace/{workspaceId}/workload/{workloadId}/run` | Workload (Opencode-Container) starten |
| `POST /api/workspace/{workspaceId}/workload/{workloadId}/stop` | Workload stoppen |
| `POST /api/workspace/{workspaceId}/workload/{workloadId}/activate` | Workload für die Session aktivieren |

## Entwicklung

```bash
go build ./...
go vet ./...
```

Struktur:

- `cmd/web` – der eigentliche Web-Server/Proxy
- `cmd/git-askpass` – git-askpass-Helper (liefert `GIT_TOKEN` an git)
- `cmd/mock` – Mock-Server für Entwicklung
- `pkg/actions` – GitLab-HTTP-Aktionen (`gitlab-http-actions.yml`)
- `pkg/auth` – Session-Management (Cookie-basiert)
- `pkg/git` – git-Wrapper (clone, commit, push)
- `pkg/gitlab` – GitLab-API-Client
- `pkg/workspace` – Workspaces, Projekte, Workloads (Domain, Service, Repository, Controller)
