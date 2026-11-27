# git-works

Web-Server und Proxy für [opencode](https://opencode.ai) – verwaltet Workspaces und Workloads
in isolierten Containern und stellt sie über GitLab-Tokens authentifiziert bereit.

## Überblick

- **Workspace-Verwaltung**: Projekte aus GitLab klonen, committen und pushen
- **Workload-Proxy**: Opencode-Instanzen (Container) werden pro Workspace gestartet und über den Server proxied
- **Authentifizierung**: wahlweise GitLab Personal Access Token (validiert gegen die GitLab-API) oder GitLab-OIDC (Authorization Code + PKCE, Refresh-Token serverseitig in der Session); Session via HttpOnly-Cookie (`gw_session`)
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
| `WORKSPACE_LOG` | `./data/workspace.v2.log` | Datei für das Workspace-Event-Log |

### GitLab

| Variable | Default | Beschreibung |
|---|---|---|
| `GITLAB_BASE_URL` | `https://gitlab.com` | Basis-URL der GitLab-Instanz (API-URL) |
| `GITLAB_ACTIONS_FILE` | `gitlab-http-actions.yml` | Konfiguration der verfügbaren GitLab-HTTP-Aktionen |
| `GITLAB_TOKEN` | – | Optionales server-seitiges GitLab-Token; wird als `config_gitlab_token` an die Workload-Aktionen übergeben |

### OIDC-Login (optional)

Ist nur aktiv, wenn `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` und eine Redirect-URI gesetzt sind. Als OIDC-Provider dient die GitLab-Instanz aus `GITLAB_BASE_URL` (Discovery über `/.well-known/openid-configuration`). In GitLab muss eine "Doorkeeper"-Anwendung mit der Redirect-URI registriert werden.

| Variable | Default | Beschreibung |
|---|---|---|
| `OIDC_CLIENT_ID` | – | Client-ID der GitLab-OIDC-Anwendung |
| `OIDC_CLIENT_SECRET` | – | Client-Secret der GitLab-OIDC-Anwendung (nur serverseitig) |
| `OIDC_SCOPES` | `openid profile email api offline_access` | Angeforderte Scopes; `offline_access` ist nötig für den Refresh-Token |
| `PUBLIC_BASE_URL` | – | Öffentliche Basis-URL des Servers, z.B. `https://works.example.com`; daraus wird die Redirect-URI abgeleitet |
| `OIDC_REDIRECT_URI` | `PUBLIC_BASE_URL + /api/auth/oidc/callback` | Redirect-URI der GitLab-Anwendung (override) |

### Git

| Variable | Default | Beschreibung |
|---|---|---|
| `SSH_ASKPASS` | `/usr/local/bin/git-askpass` | Pfad zum git-askpass-Helper (liefert den Token für git push) |
| `GIT_TOKEN` | – | GitLab-Token, der vom git-askpass-Helper gelesen wird |

### Workloads / Opencode

| Variable | Default | Beschreibung |
|---|---|---|
| `OPENCODE_REMOTE_HOST` | `127.0.0.1` | Host, unter dem die Opencode-/VSCode-Workloads erreichbar sind |
| `WORKSPACE_START_PORT` | `9000` | Startport für die Portvergabe der Workloads |
| `OPENCODE_TEMPLATE_FILE` | `opencode.json` | Template-Datei für die Opencode-Konfiguration pro Workload |
| `VSCODE_TEMPLATE_FILE` | `vscode.json` | Template-Datei für die VSCode-Konfiguration pro Workload |

### Mock-Server (`cmd/mock`)

| Variable | Default | Beschreibung |
|---|---|---|
| `SERVICE_ADRESS` | `:8999` | Listen-Adresse des Mock-Servers (Schreibweise im Code: `ADRESS`) |

Der Mock-Server stellt zusätzlich `GET /api/v4/user` (Token-Login) sowie einen kompletten OIDC-Provider (`/.well-known/openid-configuration`, `/oauth/authorize`, `/oauth/token`, `/oauth/userinfo`, `/oauth/revoke`) bereit. Lokal testbar mit `GITLAB_BASE_URL=http://localhost:8999`, `OIDC_CLIENT_ID=mock`, `OIDC_CLIENT_SECRET=mock` und `PUBLIC_BASE_URL=http://localhost:8080`. |

Hinweis: Der `Containerfile` setzt zusätzlich `PORT`, `WORKSPACE_BASE_DIR` und `STATIC_DIR` – diese werden vom Go-Code nicht gelesen. Bitte stattdessen `SERVER_ADDRESS` bzw. `DATA_DIR` setzen.

## Tasks-MCP-Server (`cmd/tasks`)

Einfacher MCP-Server (stdio), der eine persistente Task-Liste als JSON-Datei speichert, damit ein Agent (z.B. opencode) Aufgaben nach und nach abarbeiten kann.

```bash
go build -o tasks ./cmd/tasks
./tasks -file /data/tasks.json   # oder Env: TASKS_FILE=/data/tasks.json (Default: ./tasks.json)
```

Einbindung in opencode (z.B. über `OPENCODE_TEMPLATE_FILE` pro Workload in `opencode.json`):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "tasks": {
      "type": "local",
      "command": ["/usr/local/bin/tasks", "-file", "/workspace/tasks.json"]
    }
  }
}
```

### Tools

| Tool | Beschreibung |
|---|---|
| `task_add` | Neue Aufgabe anlegen (`title`, optional `description`, `priority`: `low`/`normal`/`high`); liefert die ID (z.B. `T-1`) |
| `task_list` | Aufgaben auflisten, optional gefiltert nach `status` (`todo`, `in_progress`, `done`, `all`) |
| `task_next` | Nächste offene Aufgabe holen (höchste Priorität, bei Gleichstand älteste zuerst) und auf `in_progress` setzen; `task: null` wenn keine offen ist |
| `task_update` | Felder einer Aufgabe ändern (nur übergebene Felder werden geändert) |
| `task_complete` | Aufgabe auf `done` setzen, optional mit `note` |
| `task_remove` | Aufgabe löschen |

Alle Antworten liefern neben Text auch `structuredContent` (JSON). Der Store wird bei jeder Änderung atomar (Temp-Datei + Rename) geschrieben und überlebt damit Neustarts; die Datei ist außerdem menschenlesbar und git-commitable.

Workflow für den Agenten (steht auch in den Server-Instructions): `task_next` → Aufgabe abarbeiten → `task_complete`.

```bash
go test ./cmd/tasks
```

## HTTP-Endpunkte

| Methode & Pfad | Beschreibung |
|---|---|
| `GET /health` | Healthcheck |
| `POST /api/auth/token` | Login mit GitLab-Token, setzt Session-Cookie |
| `GET /api/auth/methods` | Verfügbare Login-Methoden (`token`, `oidc`) – steuert den Umschalter auf der Loginseite |
| `GET /api/auth/oidc/login` | Startet den OIDC-Flow (Redirect zu GitLab, PKCE + State-Cookies) |
| `GET /api/auth/oidc/callback` | OIDC-Callback: Code gegen Tokens tauschen, Session anlegen, Redirect auf `/` |
| `GET /api/auth/logout` | Session löschen; bei OIDC-Sessions werden Access-/Refresh-Token bei GitLab revoked |
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
- `cmd/tasks` – MCP-Server (stdio) für persistente Task-Listen
- `pkg/actions` – GitLab-HTTP-Aktionen (`gitlab-http-actions.yml`)
- `pkg/auth` – Session-Management (Cookie-basiert)
- `pkg/git` – git-Wrapper (clone, commit, push)
- `pkg/gitlab` – GitLab-API-Client
- `pkg/workspace` – Workspaces, Projekte, Workloads (Domain, Service, Repository, Controller)
