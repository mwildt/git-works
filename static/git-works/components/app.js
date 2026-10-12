import { LitElement, html, css } from '../lit.js';
import './atomic/header.js';
import './workspace.js';
import './login.js';
import {router} from './router.js'



customElements.define('gw-page', class extends LitElement {

    static styles = css`
        header {
            display: block;
            background: linear-gradient(135deg, #4CAF50 0%, #2E7D32 100%);
            color: white;
            padding: 24px 0;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
            margin-bottom: 32px;
        }
        .content {
            max-width: 1200px;
            margin: 0 auto;
            padding: 0 24px;
        }
        
        .header {
            display: flex;
            justify-content: flex-start;
            gap: 1rem;
            align-items: center;
        }
    `;

    render() {
        return html`
            <header>
                <div class="content header">
                    <action @click=${(e) => {
                        e.preventDefault();
                        router.go('/');
                    }}><strong>Git.Works</strong></action>
                    <slot name="header"></slot>
                </div>
            </header>
            <main>
                <div class="content">
                    <slot></slot>
                </div>
            </main>
        `
    }

})

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
            case '/user/profile': return html`
                <gw-profile></gw-profile>
            `;
            default: return html`<h1>Not Found</h1>`;
        }
    }

})


customElements.define('gw-auth', class extends LitElement {
    static properties = {
        state: {}
    };

    constructor(props) {
        super(props);
        this.state = "init"
    }

    async connectedCallback() {
        super.connectedCallback();
        await this.checkSession();
        this.addEventListener('gw-login::success', () => {
            this.state = "authenticated"
        });
    }

    async checkSession() {
        try {
            const response = await fetch('/api/user');
            if (response.ok) {
                this.state = "authenticated"
            } else {
                this.state = "login"
            }
        } catch (e) {
            this.state = "login"
        }
    }

    render() {
        return html`
            ${this.state === "init" ? html`<slot name="init"></slot>` : undefined}
            ${this.state === "login" ? html`<slot name="login"></slot>` : undefined}
            ${this.state === "authenticated" ? html`<slot></slot>` : undefined}
        `
    }

})


customElements.define('gw-app', class extends LitElement {
    static properties = {
        authenticated: { type: Boolean },
        checking: { type: Boolean },
    };

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
            <gw-auth>

                <gw-page slot="init">
                    <h1>Git-Works is loading...</h1>
                </gw-page>
                
                <gw-page slot="login">
                    <gw-login slot></gw-login>
                </gw-page>
                
                <gw-page slot>
                    <div slot="header">
                        <action @click=${(e) => {
                            e.preventDefault();
                            router.go('/user/workspace');
                        }}>Arbeitsbereiche</action>
    
                        <action @click=${(e) => {
                            e.preventDefault();
                            router.go('/user/profile');
                        }}>Profil</action>
    
                    </div>
                    <gw-app-main slot></gw-app-main>
                </gw-page>
            </gw-auth>
        `;
    }
});
