import { LitElement, html, css } from '../lit.js';
import './atomic/button.js';
import './atomic/error-message.js';
import './commit-form.js';

customElements.define('gw-workspace-project', class extends LitElement {

    static properties = {
        status: {},
        project: {},
        commit: {},
        workspaceId: {},
        committing: {},
        showCommits: { type: Boolean }
    };

    static styles = css`
        .commits {
            margin: 8px 0;
            padding: 12px;
            background-color: #f9f9f9;
            border-radius: 8px;
        }
        .commits-title {
            font-weight: bold;
            margin-bottom: 8px;
        }
        .commit-list {
            list-style: none;
            margin: 0;
            padding: 0;
            display: flex;
            flex-direction: column;
            gap: 4px;
        }
        .commit-item {
            font-size: 13px;
            display: flex;
            gap: 8px;
        }
        .commit-short {
            font-family: monospace;
            color: #555;
        }
    `;

    constructor() {
        super();
        this.commit = undefined;
        this.status = {};
        this.committing = false;
        this.showCommits = false;
    }

    async connectedCallback() {
        super.connectedCallback()
        this.loadStatus();
    }


    render() {

        return html`
            <h4>${this.project.git_url} &#8594 ${this.project.path} </h4>
            <pre>${JSON.stringify(this.project)}</pre>
            <gw-row-footer>
                <div>
                    <div> branch: ${this.project.branch} 
                        | path: ${this.project.path} 
                        | ${this.project.id}
                    </div>
                    ${this.status.initialized
                            ? html`<gw-badge ok>Initialisiert</gw-badge>`
                            : html`<gw-badge warn>Nicht initialisiert</span>`}
                    ${this.status.initialized
                            ? (this.status.has_changes
                                    ? html`<gw-badge warn>Änderungen vorhanden</span>`
                                    : html`<gw-badge ok>Keine Änderungen</gw-badge >`)
                            : undefined}
                </div>

                <div slot="actions">
                    <gw-button @click=${() => this.init()}>Init</gw-button>
                    <gw-button @click=${() => this.toggleCommits()}>Commits</gw-button>
                    ${this.status.has_changes
                            ? html`<gw-button danger @click=${() => this.commit = {}}>Commit</gw-button>`
                            : undefined}
                    <gw-button danger @click=${() => this.removeProject()}>Löschen</gw-button>
                </div>
            </gw-row-footer>

            ${this.commit
                    ? html`<gw-commit-form .project=${this.project} .committing=${this.committing} .commitBranch=${this.project.branch}
                           @gw-commit-form::commit=${(e) => this.handleCommit(e.detail)}
                           @gw-commit-form::push=${(e) => this.handlePush(e.detail)}
                           @gw-commit-form::cancel=${() => this.commit = undefined}
                    ></gw-commit-form>`
                    : undefined}

            ${this.showCommits
                    ? this.renderCommits()
                    : undefined}
        `
    }

    renderCommits() {
        const commits = this.status.commits || [];
        return html`
            <div class="commits">
                <div class="commits-title">Commits auf HEAD, nicht im Branch ${this.project.branch}</div>
                ${commits.length > 0
                        ? html`<ul class="commit-list">
                            ${commits.map(commit => html`<li class="commit-item">
                                <span class="commit-short">${commit.short}</span>
                                <span>${commit.author}</span>
                                <span>${commit.message}</span>
                            </li>`)}
                        </ul>`
                        : html`<span>Keine Commits vorhanden.</span>`}
            </div>
        `;
    }

    toggleCommits() {
        if (!this.showCommits) {
            this.loadStatus();
        }
        this.showCommits = !this.showCommits;
    }


    async handleCommit(data) {
        this.committing = true;
        try {
            const response = await fetch(`/api/workspace/${this.workspaceId}/project/${this.project.id}/commit`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ message: data.message })
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            this.commit = undefined
            await this.loadStatus();
        } catch (error) {
            this.dispatchEvent(new CustomEvent('gw-workspace::commit-error', {
                detail: { message: 'Commit fehlgeschlagen: ' + error.message },
                bubbles: true,
                composed: true
            }));
        } finally {
            this.committing = false;
        }
    }

    async handlePush(data) {
        this.committing = true;
        try {
            const response = await fetch(`/api/workspace/${this.workspaceId}/project/${this.project.id}/push`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ branch: data.branch })
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            this.commit = undefined
            await this.loadStatus();
        } catch (error) {
            this.dispatchEvent(new CustomEvent('gw-workspace::commit-error', {
                detail: { message: 'Push fehlgeschlagen: ' + error.message },
                bubbles: true,
                composed: true
            }));
        } finally {
            this.committing = false;
        }
    }

    async loadStatus() {
        if (!this.project) {
            return
        }
        const response = await fetch(`/api/workspace/${this.workspaceId}/project/${this.project.id}/status`);
        if (!response.ok) {
            alert('Status fehlgeschlagen')
            throw new Error(`HTTP ${response.status}`);
        }
        this.status = await response.json();
    }

    async init() {
        const response = await fetch(`/api/workspace/${this.workspaceId}/project/${this.project.id}/init`, {
            method: "POST"
        });
        if (!response.ok) {
            alert('Init fehlgeschlagen')
            throw new Error(`HTTP ${response.status}`);
        }

        this.dispatchEvent(new CustomEvent('gw-workspace-project::init-success', {
            detail: this.workload,
            bubbles: true,
            composed: true,
        }));
    }



    async removeProject(project) {
        const delResponse = await fetch(`/api/workspace/${this.workspaceId}/project/${this.project.id}`, {
            method: "DELETE"
        });
        if (!delResponse.ok) {
            alert('Entfernen fehlgeschlagen')
            throw new Error(`HTTP ${delResponse.status}`);
        }

        this.dispatchEvent(new CustomEvent('gw-workspace-project::delete-success', {
            detail: this.project,
            bubbles: true,
            composed: true,
        }));
    }

})

customElements.define('gw-workspace-workload', class extends LitElement {

    static properties = {
        status: {},
        workload: {},
        workspaceId: {}
    };

    constructor() {
        super();
        this.status = {}

    }

    async connectedCallback() {
        super.connectedCallback()
        this.loadStatus();
    }

    render() {
        return html`
            <h4>${this.workload.name}</h4>
            <pre>${JSON.stringify(this.workload)}</pre>
            <gw-row-footer>
                ${this.status && this.status.status == "up"
                        ? html`<gw-badge ok>UP</gw-badge>`
                        : this.status.status == "down"
                                ? html`<gw-badge warn>Down</gw-badge>`
                                : html`<gw-badge error>Unknown</gw-badge>`}\`
                <div slot="actions">
                    <gw-button @click=${() => this.loadStatus()}>Status</gw-button>
                    <gw-button @click=${() => this.init()}>Init</gw-button>
                    ${this.status && this.status.status == "up" 
                            ? html`
                                <gw-button primary @click=${() => this.activate()}>Activate</gw-button>
                                <gw-button danger @click=${() => this.stop()}>Stop</gw-button>
                            `
                            : undefined}
                    ${this.status && this.status.status == "down"
                            ? html`<gw-button @click=${() => this.run()}>Start</gw-button>`
                            : undefined}
                 
                    
                    
<!--                    <gw-button danger @click=${() => this.remove()}>Löschen</gw-button>-->
                </div>
            </gw-row-footer>
        `
    }


    async loadStatus() {
        if (!this.workload) {
            return
        }
        const response = await fetch(`/api/workspace/${this.workspaceId}/workload/${this.workload.id}/status`);
        if (!response.ok) {
            alert('Status fehlgeschlagen')
            throw new Error(`HTTP ${response.status}`);
        }
        this.status = await response.json();
    }
    async remove() {
        const response = await fetch(`/api/workspace/${this.workspaceId}/workload/${this.workload.id}`, {
            method: "DELETE"
        });
        if (!response.ok) {
            alert('Delete fehlgeschlagen')
            throw new Error(`HTTP ${response.status}`);
        }

        this.dispatchEvent(new CustomEvent('gw-workspace-workload::delete-success', {
            detail: this.workload,
            bubbles: true,
            composed: true,
        }));
    }

    async run() {
        const response = await fetch(`/api/workspace/${this.workspaceId}/workload/${this.workload.id}/run`, {
            method: "POST"
        });
        if (!response.ok) {
            alert('Run fehlgeschlagen')
            throw new Error(`HTTP ${response.status}`);
        }

        this.dispatchEvent(new CustomEvent('gw-workspace-workload::run-success', {
            detail: this.workload,
            bubbles: true,
            composed: true,
        }));
        this.loadStatus();
    }
    async stop() {
        const response = await fetch(`/api/workspace/${this.workspaceId}/workload/${this.workload.id}/stop`, {
            method: "POST"
        });
        if (!response.ok) {
            alert('Stop fehlgeschlagen')
            throw new Error(`HTTP ${response.status}`);
        }

        this.dispatchEvent(new CustomEvent('gw-workspace-workload::stop-success', {
            detail: this.workload,
            bubbles: true,
            composed: true,
        }));
        this.loadStatus();
    }

    async activate() {
        const response = await fetch(`/api/workspace/${this.workspaceId}/workload/${this.workload.id}/activate`, {
            method: "POST",
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                location: window.location.href.replace(window.location.origin, '')
            })
        });
        if (!response.ok) {
            alert('Activate fehlgeschlagen')
            throw new Error(`HTTP ${response.status}`);
        }
        location.reload()
    }

    async init() {
        const response = await fetch(`/api/workspace/${this.workspaceId}/workload/${this.workload.id}/init`, {
            method: "POST"
        });
        if (!response.ok) {
            alert('Entfernen fehlgeschlagen')
            throw new Error(`HTTP ${response.status}`);
        }

        this.dispatchEvent(new CustomEvent('gw-workspace-workload::init-success', {
            detail: this.workload,
            bubbles: true,
            composed: true,
        }));
    }

})

customElements.define('gw-workspace-init', class extends LitElement {

    static properties = {
        workspaceId: {},
        running: {}
    };

    render() {
        if (!this.running) {
            return html`<gw-button @click=${() => this.start()}>Workspace initialisieren</gw-button>`
        }
        return html`
            <gw-dialog>
                    ......
            </gw-dialog>
        `;
    }

    async start() {
        this.running = true
        const response = await fetch(`/api/workspace/${this.workspaceId}/init`, {
            method: "POST"
        });
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        this.dispatchEvent(new CustomEvent('gw-workspace-init::success', {
            detail: this.project,
            bubbles: true,
            composed: true,
        }));
        this.running = false
    }

})

customElements.define('gw-workspace', class extends LitElement {
    static properties = {
        workspaceId: {},
        workspace: {},
        state: {}
    };

    constructor() {
        super();
        this.state = "loading"
        this.addEventListener('gw-add-project::success', async () => {
            await this.reload();
        });
        this.addEventListener('gw-workspace-project::delete-success', async () => {
            await this.reload();
        });
    }

    willUpdate(changed) {
        if (changed.has('workspaceId') && changed["workspaceId"]!== this.workspaceId) {
            this.reload();
        }
    }

    render() {
        if (this.state === "loading") {
            return html`<h1>...loading</h1>`
        }

        if (this.state === "notFound") {
            return html`<h1>Der gesuchte workspace scheint leider nicht zu existieren</h1>`
        }

        if (this.state === "error") {
            return html`<h1>Beim Laden ist leider in Fehler aufgetreten.</h1>`
        }

        return html`
            <h2>${this.workspace.name}</h2>
           
            <div>
                Owner ${this.workspace.owner}, am: ${this.workspace.created}
            </div>
            <gw-workspace-init .workspaceId=${this.workspace.id}></gw-workspace-init>
            <div>
                <h3>Workloads</h3>
                ${this.workspace.workloads.length > 0
                        ? this.workspace.workloads.map(workload => html`<gw-row>
                            <gw-workspace-workload .workspaceId=${this.workspace.id} .workload=${workload}></gw-workspace-workload>
                        </gw-row>`)
                        : html`<a>Bisher sind keine Workloads definiert.</a>`}
            </div>
            <div>
                <h3>Projekte</h3>
                <gw-add-project .workspaceId=${this.workspace.id}></gw-add-project>
                ${this.workspace.projects.length > 0 
                    ? this.workspace.projects.map(project => html`<gw-row>
                            <gw-workspace-project .workspaceId=${this.workspace.id} .project=${project}></gw-workspace-project>
                        </gw-row>`)
                    : html`<a>Bisher sind keine Projekte vorhanden.</a>`}
            </div>
        `;
    }

    closeCommitForm() {
        this.commitProject = null;
    }

    async reload() {
        if (this.workspaceId) {
            const response = await fetch(`/api/workspace/${this.workspaceId}`);
            if (response.status === 404) {
                this.state = 'notFound';
                return;
            }
            if (!response.ok) {
                this.state = 'error';
                return;
            }
            this.workspace = await response.json();
            this.state = 'loaded';

        }
    }

});