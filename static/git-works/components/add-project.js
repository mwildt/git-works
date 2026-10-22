import { LitElement, html, css } from '../lit.js';
import './atomic/button.js';
import './atomic/input.js';
import './atomic/selectable.js';

customElements.define('gw-add-project', class extends LitElement {

    static properties = {
        workspaceId: {},
        model: {},
        projects: {},
        branches: {},
        searchTerm: {},
        nextPage: { type: Number },
        searchTimer: {},
        loading: { type: Boolean },
    };

    static styles = css`
        :host {
            display: block
        }
    
        .section {
            margin-bottom: 20px;
        }

        .description {
            color: #666;
            font-size: 14px;
            margin-bottom: 8px;
            display: block;
        }

        .actions {
            display: flex;
            gap: 12px;
            margin-top: 24px;
        }

        .cancel-btn {
            --button-type: secondary;
        }

        pre {
            background-color: #f5f5f5;
            padding: 12px;
            border-radius: 8px;
            overflow-x: auto;
            font-size: 12px;
            margin: 16px 0;
            color: #333;
        }

        .no-results {
            padding: 16px;
            color: #999;
            font-size: 14px;
            text-align: center;
        }

        .selectable-list {
            max-height: 200px;
            overflow-y: auto;
            border: 1px solid #e0e0e0;
            border-radius: 8px;
            margin: 8px 0;
        }

        .selectable-list::-webkit-scrollbar {
            width: 8px;
        }

        .selectable-list::-webkit-scrollbar-track {
            background: #f1f1f1;
            border-radius: 4px;
        }

        .selectable-list::-webkit-scrollbar-thumb {
            background: #c1c1c1;
            border-radius: 4px;
        }

        .selectable-list::-webkit-scrollbar-thumb:hover {
            background: #a1a1a1;
        }
    `;

    constructor() {
        super();
        this.searchTerm = '';
        this.nextPage = 0;
        this.searchTimer = undefined;
        this.loading = false;
    }

    async connectedCallback() {
        super.connectedCallback();
    }

    render() {
        if (this.model === undefined) {
            return html`<gw-button @click=${() => this.start()}>Projekt anlegen</gw-button>`
        }

        return html`
            <gw-modal>
                <h3>Neus Projekt hinzufügen</h3>
                
                <div class="section">
                    <h4>Gitlab Repository auswählen</h4>
                    <span class="description">Mindestens 3 Zeichen eingeben, um in Gitlab zu suchen</span>
                    <gw-input 
                        .value=${this.searchTerm}
                        placeholder="z.B. my-project"
                        @input=${(e) => this.onSearchInput(e)}
                    >
                        <span slot="label">Suche</span>
                    </gw-input>
                    ${this.projects === undefined
                        ? undefined
                        : html`
                            <div class="selectable-list">
                                ${this.projects.map(project => html`
                                    <gw-selectable 
                                        ?selected=${project.id === this.model.project?.id}
                                        @click=${() => this.selectProject(project)}
                                    >
                                        <span slot="primary">${project.name}</span>
                                        <span slot="secondary">${project.web_url}</span>
                                    </gw-selectable>
                                `)}
                                ${this.projects.length === 0
                                    ? html`<div class="no-results">Keine Projekte gefunden</div>`
                                    : undefined}
                            </div>
                            ${this.nextPage > 0
                                ? html`<gw-button type="secondary" ?disabled=${this.loading} @click=${() => this.loadMore()}>Mehr laden</gw-button>`
                                : undefined}
                        `}
                </div>

                ${this.model.project !== undefined
                    ? html`
                        <h4>Branch auswählen</h4>
                        <div class="selectable-list">
                            ${this.branches && this.branches.map(branch => html`
                                <gw-selectable 
                                    ?selected=${branch.name === this.model.branch?.name}
                                    @click=${() => this.selectBranch(branch)}
                                >
                                    <span slot="primary">${branch.name}</span>
                                </gw-selectable>
                            `)}
                        </div>

                        <div class="section">
                            <h4>Zielpfad festlegen</h4>
                            <span class="description">Hier wird das Projekt ausgecheckt</span>
                            <gw-input .value=${this.model.path || ''} @input=${(e) => this.model = { ...this.model, path: e.target.value }} >
                                <span slot="label">Pfad</span>
                            </gw-input>
                        </div>`
                        : undefined}
                <div class="actions">
                    <gw-button type="secondary" @click=${() => this.model = undefined}>Abbrechen</gw-button>
                    ${this.valid() ? html`<gw-button primary @click=${() => this.submit()}>Hinzufügen</gw-button>` : undefined}
                </div>
            </gw-modal>
        `;
    }

    valid() {
        return this.model
        && !this.empty(this.model.path)
        && this.model.branch != undefined
        && this.model.project != undefined
    }

    empty(v) {
        return v === undefined || v.length == 0
    }
    async selectBranch(branch) {
        this.model = { ...this.model, branch };
    }

    onSearchInput(e) {
        const value = e.detail?.value ?? e.target.value;
        this.searchTerm = value;
        if (this.searchTimer !== undefined) {
            clearTimeout(this.searchTimer);
        }
        if (value.trim().length < 3) {
            this.projects = undefined;
            this.nextPage = 0;
            return;
        }
        this.searchTimer = setTimeout(() => this.searchProjects(), 300);
    }

    async searchProjects() {
        if (this.searchTerm.trim().length < 3) {
            return;
        }
        this.loading = true;
        this.projects = undefined;
        this.nextPage = 0;
        if (this.model.project !== undefined) {
            this.model = { ...this.model, project: undefined };
            this.branches = undefined;
        }
        const result = await this.fetchProjects(1);
        this.loading = false;
        if (result) {
            this.projects = result.projects ?? [];
            this.nextPage = result.next_page ?? 0;
        }
    }

    async loadMore() {
        if (this.nextPage < 1) {
            return;
        }
        this.loading = true;
        const page = this.nextPage;
        const result = await this.fetchProjects(page);
        this.loading = false;
        if (result) {
            const existing = this.projects ?? [];
            const existingIds = new Set(existing.map(p => p.id));
            this.projects = [...existing, ...(result.projects ?? []).filter(p => !existingIds.has(p.id))];
            this.nextPage = result.next_page ?? 0;
        }
    }

    async fetchProjects(page) {
        const search = encodeURIComponent(this.searchTerm.trim());
        const response = await fetch(`/api/gitlab/project?search=${search}&page=${page}`);
        if (!response.ok) {
            return undefined;
        }
        return await response.json();
    }

    async selectProject(project) {
        this.model = { ...this.model, project, path: project.path_with_namespace };
        this.branches = undefined;
        const response = await fetch(`/api/gitlab/project/${project.id}/branches`);
        this.branches = await response.json();
    }

    async start() {
        this.model = {};
        this.searchTerm = '';
        this.projects = undefined;
        this.nextPage = 0;
    }

    async submit() {
        const response = await fetch(`/api/workspace/${this.workspaceId}/project`, {
            method: "POST",
            body: JSON.stringify(this.model),
        });
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        this.dispatchEvent(new CustomEvent('gw-add-project::success', {
            detail: this.model,
            bubbles: true,
            composed: true,
        }));

        this.model = undefined;
    }
});