import { LitElement, html, css } from '../lit.js';
import './atomic/button.js';
import './atomic/input.js';

customElements.define('gw-login', class extends LitElement {
    static properties = {
        error: { type: String },
        loading: { type: Boolean },
        mode: { type: String },
        methods: { state: true },
    };

    static styles = css`
        :host {
            display: block;
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
        }
        .card {
            max-width: 400px;
            margin: 48px auto 0 auto;
            padding: 32px;
            background-color: white;
            border-radius: 12px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
        }
        h3 {
            color: #2E7D32;
            font-size: 20px;
            font-weight: 600;
            margin: 0 0 8px 0;
        }
        p {
            color: #666;
            font-size: 14px;
            margin-bottom: 20px;
        }
        .modes {
            display: flex;
            gap: 8px;
            margin-bottom: 20px;
            background-color: #f5f5f5;
            border-radius: 8px;
            padding: 4px;
        }
        .modes button {
            flex: 1;
            border: none;
            background: transparent;
            padding: 8px 12px;
            border-radius: 6px;
            font-size: 14px;
            cursor: pointer;
            color: #666;
        }
        .modes button.active {
            background-color: white;
            color: #2E7D32;
            font-weight: 600;
            box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
        }
        gw-input {
            display: block;
            margin-bottom: 20px;
        }
        .error {
            background-color: #ffebee;
            color: #c62828;
            padding: 10px 14px;
            border-radius: 8px;
            font-size: 14px;
            margin-bottom: 16px;
            border-left: 4px solid #f44336;
        }
        .actions {
            display: flex;
            justify-content: flex-end;
        }
    `;

    constructor() {
        super();
        this.error = '';
        this.loading = false;
        this.mode = 'token';
        this.methods = { token: true, oidc: false };
    }

    async connectedCallback() {
        super.connectedCallback();
        try {
            const response = await fetch('/api/auth/methods');
            if (response.ok) {
                this.methods = await response.json();
                if (!this.methods.token && this.methods.oidc) {
                    this.mode = 'oidc';
                }
            }
        } catch (e) {
            // ohne /api/auth/methods bleibt der Token-Login sichtbar
        }
    }

    render() {
        const showModes = this.methods.oidc && this.methods.token;
        return html`
            <div class="card">
                <h3>Anmeldung erforderlich</h3>
                ${showModes ? html`
                    <div class="modes">
                        <button class=${this.mode === 'token' ? 'active' : ''} @click=${() => this.mode = 'token'}>Access Token</button>
                        <button class=${this.mode === 'oidc' ? 'active' : ''} @click=${() => this.mode = 'oidc'}>GitLab SSO</button>
                    </div>
                ` : undefined}
                ${this.error ? html`<div class="error">${this.error}</div>` : undefined}
                ${this.mode === 'oidc' && this.methods.oidc ? html`
                    <p>Melde dich mit deinem GitLab-Account an. Du wirst zu GitLab weitergeleitet.</p>
                    <div class="actions">
                        <gw-button ?disabled=${this.loading} @click=${this.sso}>
                            ${this.loading ? 'Weiterleitung...' : 'Mit GitLab anmelden'}
                        </gw-button>
                    </div>
                ` : html`
                    <p>Bitte gib deinen Access-Token ein, um fortzufahren.</p>
                    <gw-input type="password" name="password" autocomplete="current-password" .value=${this.token || ''} @input=${(e) => this.token = e.target.value} @keydown=${(e) => e.key === 'Enter' && this.submit(e)}>
                        <span slot="label">Access Token</span>
                    </gw-input>
                    <div class="actions">
                        <gw-button ?disabled=${this.loading} @click=${this.submit}>
                            ${this.loading ? 'Anmelden...' : 'Anmelden'}
                        </gw-button>
                    </div>
                `}
            </div>
        `;
    }

    sso(event) {
        if (event) event.preventDefault();
        this.loading = true;
        this.error = '';
        location.href = '/api/auth/oidc/login';
    }

    async submit(event) {
        if (event) event.preventDefault();
        this.loading = true;
        this.error = '';
        try {
            const response = await fetch('/api/auth/token', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ token: this.token }),
            });
            if (!response.ok) {
                this.error = 'Anmeldung fehlgeschlagen. Bitte Token prüfen.';
                return;
            }
            this.dispatchEvent(new CustomEvent('gw-login::success', { bubbles: true, composed: true }));
        } catch (e) {
            this.error = 'Netzwerkfehler: ' + e.message;
        } finally {
            this.loading = false;
        }
    }
});
