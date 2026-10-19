import { LitElement, html, css } from '../lit.js';
import './atomic/button.js';
import './atomic/input.js';
import './atomic/selectable.js';

customElements.define('gw-add-project', class extends LitElement {

    static properties = {
        workspaceId: {},
        model: {},
        projects: {},
        branches: {}
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
    }

    async connectedCallback() {
        super.connectedCallback();
    }

    render() {
        if (this.model === undefined) {
            return html`<gw-button @click=${() => this.start()}>Projekt anlegen</gw-button>`
        }

        return html`
            <gw-dialog>
                <h3>Neus Projekt hinzufügen</h3>
                
                <div class="section">
                    <h4>Gitlab Repository auswählen</h4>
                    <div class="selectable-list">
                        ${this.projects && this.projects.map(project => html`
                            <gw-selectable 
                                ?selected=${project.id === this.model.project?.id}
                                @click=${() => this.selectProject(project)}
                            >
                                <span slot="primary">${project.name}</span>
                                <span slot="secondary">${project.web_url}</span>
                            </gw-selectable>
                        `)}
                    </div>
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
                        </div>

                        <div class="actions">
                            <gw-button @click=${() => this.submit()}>Hinzufügen</gw-button>
                            <gw-button type="secondary" @click=${() => this.model = undefined}>Abbrechen</gw-button>
                        </div>
                    `
                    : undefined}
            </gw-dialog>
        `;
    }

    async selectBranch(branch) {
        this.model = { ...this.model, branch };
    }

    async selectProject(project) {
        this.model = { ...this.model, project, path: project.path_with_namespace };
        this.branches = undefined;
        const response = await fetch(`/api/gitlab/project/${project.id}/branches`);
        this.branches = await response.json();
    }

    async start() {
        this.model = {};
        const response = await fetch('/api/gitlab/project');
        this.projects = await response.json();
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