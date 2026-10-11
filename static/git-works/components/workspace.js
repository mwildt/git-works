import { LitElement, html, css } from '../lit.js';
import './atomic/button.js';
import './atomic/error-message.js';
import './commit-form.js';

customElements.define('gw-workspace-status', class extends LitElement {
    static properties = {
        status: {},
        wsid: {}
    };

    constructor(props) {
        super(props);
        this.status = {}
        this.reload()
    }

    async connectedCallback() {
        super.connectedCallback()
        this.reload()
    }


    render() {
        return html`Status: ${this.status.status}`
    }

    async reload() {
        if (this.wsid) {
            const response = await fetch(`/api/workspace/${this.wsid}/status`);
            this.status = await response.json();
            console.warn(this.status)
        }
    }

})


customElements.define('gw-workspace-oc-control', class extends LitElement {
    static properties = {
        wsid: {}
    };

    constructor(props) {
        super(props);
    }

    async connectedCallback() {
        super.connectedCallback()
        this.reload()
    }


    render() {
        return html`
            <gw-button @click=${() => this.start()}>Start</gw-button>
            <gw-button @click=${() => this.stop()}>Stop</gw-button>
            <gw-button @click=${() => this.activate()}>Aktivieren</gw-button>
        `
    }

    async activate() {
        const resp = await fetch(`/api/workspace/${this.wsid}/activate`, {
            method: "POST"
        });
        if (!resp.ok) {
            alert('Aktivieren fehlgeschlagen')
            throw new Error(`HTTP ${resp.status}`);
        } else {
            location.reload()
        }
    }
    async start() {
        const resp = await fetch(`/api/workspace/${this.wsid}/oc-up`, {
            method: "POST"
        });
        if (!resp.ok) {
            alert('Start fehlgeschlagen')
            throw new Error(`HTTP ${resp.status}`);
        }
    }

    async stop() {
        const resp = await fetch(`/api/workspace/${this.wsid}/oc-down`, {
            method: "POST"
        });
        if (!resp.ok) {
            alert('Stop fehlgeschlagen')
            throw new Error(`HTTP ${resp.status}`);
        }
    }


})

customElements.define('gw-workspace-project-status', class extends LitElement {
    static properties = {
        status: {},
        wsid: {},
        projectId: {}
    };

    constructor(props) {
        super(props);
        this.status = {}
        this.reload()
    }

    async connectedCallback() {
        super.connectedCallback()
        this.reload()
    }


    render() {
        return html`Status: ${JSON.stringify(this.status)}`
    }

    async reload() {
        if (this.wsid) {
            const response = await fetch(`/api/workspace/${this.wsid}/project/${this.projectId}/status`);
            this.status = await response.json();
            console.warn(this.status)
        }
    }

})

customElements.define('gw-workspace-project', class extends LitElement {

    static properties = {
        project: {},
        wsid: {}
    };

    render() {
        return html`
            <h4>${this.project.git_url} &#8594 ${this.project.path} </h4>
            
            <div>
                <gw-workspace-project-status .wsid=${this.wsid} .projectId=${this.project.id}></gw-workspace-project-status>
            </div>
            
            <div>
                <span>branch: ${this.project.branch} 
                    | path: ${this.project.path} 
                    | ${this.project.id} 
                    | <gw-action @click=${() => this.removeProject()}>Entfernen</gw-action>
            </div>
        `
    }

    async removeProject(project) {
        const delResponse = await fetch(`/api/workspace/${this.wsid}/project/${this.project.id}`, {
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


customElements.define('gw-workspace-init', class extends LitElement {

    static properties = {
        wsid: {},
        running: {}
    };

    render() {
        if (!this.running) {
            return html`<gw-button @click=${() => this.start()}>Workspace initialisiern</gw-button>`
        }
        return html`
            <gw-dialog>
                    ......
            </gw-dialog>
        `;
    }

    async start() {
        this.running = true
        const response = await fetch(`/api/workspace/${this.wsid}/init`, {
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
        wsid: {},
        workspace: {},
    };

    constructor() {
        super();

        this.addEventListener('gw-add-project::success', async () => {
            await this.reload();
        });
        this.addEventListener('gw-workspace-project::delete-success', async () => {
            await this.reload();
        });
    }

    willUpdate(changed) {
        if (changed.has('wsid') && changed["wsid"]!== this.wsid) {
            this.reload();
        }
    }

    render() {
        if (!this.workspace) {
            return html`<h1>...loading</h1>`
        }

        return html`
            <h2>${this.workspace.name}</h2>
            <div>
                <gw-workspace-status .wsid=${this.workspace.id}></gw-workspace-status>
            </div>
            <div>
                Owner ${this.workspace.owner}, am: ${this.workspace.created}
            </div>
            <gw-workspace-oc-control .wsid=${this.workspace.id}></gw-workspace-oc-control>
            <gw-workspace-init .wsid=${this.workspace.id}></gw-workspace-init>
            <div>
                <h3>Projekte</h3>
                <gw-add-project .wsid=${this.workspace.id}></gw-add-project>
                ${this.workspace.projects.length > 0 
                    ? this.workspace.projects.map(project => html`<gw-row>
                            <gw-workspace-project .wsid=${this.workspace.id} .project=${project}></gw-workspace-project>
                        </gw-row>`)
                    : html`<a>Bisher sind keine Projekte vorhanden</a>`}
            </div>
        `;
    }

    closeCommitForm() {
        this.commitProject = null;
    }

    async reload() {
        if (this.wsid) {
            const response = await fetch(`/api/workspace/${this.wsid}`);
            this.workspace = await response.json();
        }
    }

});