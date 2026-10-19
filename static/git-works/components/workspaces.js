import { LitElement, html, css } from '../lit.js';
import './atomic/button.js';
import './atomic/error-message.js';
import './commit-form.js';
import {router} from './router.js'


customElements.define('gw-create-workspace', class extends LitElement {
    static properties = {
        model: {},
    };

    render() {
        if (this.model === undefined) {
            return html`<gw-button @click=${() => this.start()}>Workspace anlegen</gw-button>`
        }
        return html`
            <gw-dialog>
                <h4>Neuen Workspace anlegen</h4>
                
                <p>... Der name wird erstmal zufällig erzeugt.</p>
                
                <div class="actions">
                    <gw-button @click=${() => this.submit()}>Hinzufügen</gw-button>
                    <gw-button type="secondary" @click=${() => this.model = undefined}>Abbrechen</gw-button>
                </div>
            </gw-dialog>
        `;
    }

    async start() {
        this.model = {};
    }

    async submit() {
        const response = await fetch(`/api/workspace`, {
            method: "POST",
            body: JSON.stringify(this.model),
        });
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        this.dispatchEvent(new CustomEvent('gw-create-workspace::success', {
            detail: this.model,
            bubbles: true,
            composed: true,
        }));
        this.model = undefined;
    }

})

customElements.define('gw-workspaces', class extends LitElement {
    static properties = {
        workspaces: {}
    };

    static styles = css`

    `;

    constructor() {
        super();
        this.workspaces = []
        this.addEventListener('gw-create-workspace::success', async () => {
            await this.reload();
        });
    }

    async connectedCallback() {
        super.connectedCallback();
        await this.reload();
    }

    render() {
        return html`
            <h4>Deine Arbeitsbereiche</h4>
            <gw-create-workspace></gw-create-workspace>
            <div>
                ${this.workspaces.map(workspace => html`
                   
                    <gw-row>
                        <h3><a href="#/workspace?workspaceId=${workspace.id}" @click=${()=> {
                                router.go('/user/workspace', {workspaceId: workspace.id});
                        }}>${workspace.name}</a></h3>
                        <small>Erstellt von ${workspace.owner} | erstellt am ${workspace.created} | ${workspace.id} |  <gw-action @click="${() => this.delete(workspace)}">Entfernen</gw-action></small>
                    </gw-row>
                `)}
            </div>
        `;
    }


    async reload() {
        const response = await fetch('/api/workspace');
        this.workspaces = await response.json();
    }

    async delete(workspace) {
        const delResponse = await fetch(`/api/workspace/${workspace.id}`, {
            method: "DELETE"
        });
        if (!delResponse.ok) {
            throw new Error(`HTTP ${delResponse.status}`);
        }
        await this.reload();
    }
});