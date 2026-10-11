import { LitElement, html, css } from '../lit.js';
import './atomic/header.js';
import './workspace.js';
import './login.js';
import {router} from './router.js'

customElements.define('gw-app-main', class extends LitElement {
    static properties = {
        path: { state: true },
    };

    connectedCallback() {
        super.connectedCallback();
        this.path = router.path;
        this._unsubscribe = router.subscribe((nav) => {
            console.warn("setpath")
            this.path = nav.path;          // Property-Zuweisung triggert Re-Render
        });
    }

    disconnectedCallback() {
        super.disconnectedCallback();
        this._unsubscribe();
    }

    render() {
        console.warn("path ", this.path)
        switch (this.path) {
            case '/': return html`
                <h1 class="page-title">Was geht App?</h1>
                <p class="page-subtitle">Durchsuche deine Git-Projekte und erstelle <a
                        href="/static#/user/workspace" @click=${(e) => {
                    e.preventDefault();
                    router.go('/user/workspace');
                }}>Arbeitsbereiche</a>, in denen du dann mit Opencode arbeiten kannst.</p>
            `;
            case '/workspace': return html`
                <gw-workspace .wsid=${router.query.wsid}></gw-workspace>
            `;
            case '/user/workspace': return html`
                <gw-workspaces></gw-workspaces>
            `;
            default: return html`<h1>Not Found</h1>`;
        }
    }

})

customElements.define('gw-app', class extends LitElement {
    static properties = {
        authenticated: { type: Boolean },
        checking: { type: Boolean },
    };

    static styles = css`
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        .page-title {
            color: #2E7D32;
            margin-bottom: 8px;
            font-size: 24px;
            font-weight: 600;
        }
        .page-subtitle {
            color: #666;
            font-size: 16px;
            margin-bottom: 24px;
        }
    `;

    constructor() {
        super();
        this.authenticated = false;
        this.checking = true;
    }

    async connectedCallback() {
        super.connectedCallback();
        await this.checkSession();
        this.addEventListener('gw-login::success', () => {
            this.authenticated = true;
        });
    }

    async checkSession() {
        this.checking = true;
        try {
            const response = await fetch('/api/user');
            this.authenticated = response.ok;
        } catch (e) {
            this.authenticated = false;
        } finally {
            this.checking = false;
        }
    }

    render() {
        return html`
            <gw-header></gw-header>
            <div class="container">
                ${this.authenticated
                    ? html`
                        <gw-app-main></gw-app-main>
                    `
                    : html`<gw-login></gw-login>`}
            </div>
        `;
    }
});
