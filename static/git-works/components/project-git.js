import { LitElement, html, css } from '../lit.js';
import './atomic/button.js';
import './atomic/input.js';
import './atomic/error-message.js';

customElements.define('gw-project-git', class extends LitElement {

    static properties = {
        project: { },
        status: { },
        workspaceId: {},
        committing: { },
        commitMessage: { },
        commitBranch: {},
        commitError: { }
    };

    static styles = css`
        :host {
            display: block;
        }

        .commit-form {
            display: flex;
            flex-direction: column;
            gap: 8px;
            padding: 12px;
            margin: 8px 0;
        }

        .commit-form-actions {
            display: flex;
            justify-content: flex-end;
            gap: 8px;
            align-items: center;
        }

        .commit-error {
            color: #c62828;
            font-size: 13px;
        }

        .remove-btn {
            color: #f44336;
            cursor: pointer;
            font-size: 12px;
            text-decoration: underline;
            transition: color 0.2s ease;
        }

        .remove-btn:hover {
            color: #d32f2f;
        }
    `;

    constructor() {
        super();
        this.project = null;
        this.status = undefined
        this.commitMessage = '';
        this.commitBranch = '';
        this.error = null;
    }

    async connectedCallback() {
        super.connectedCallback()
        this.loadStatus();
    }

    render() {
        if (!this.project) return undefined;

        const commits = this.status.commits || [];
        const showMessage = this.status.has_changes
        const showBranch = this.status.has_changes || commits.length > 0

        return html`
            
            <h4>${this.project.path}</h4>

            ${this.error
                    ? html`<span class="commit-error">${this.error}</span>`
                    : undefined}

            <div>

                ${showMessage ? html`<gw-input
                        placeholder="Commit Message" .value=${this.commitMessage}
                        @change=${e => this.commitMessage = e.detail.value}
                ></gw-input>` : undefined }
                
                ${commits.length > 0 
                    ? html`
                        <h4>Vorhandene Commits</h4>
                        <ul class="commit-list">
                            ${commits.map(commit => html`<li class="commit-item">
                                <span class="commit-short">${commit.short}</span>
                                <span>${commit.author}</span>
                                <span>${commit.message}</span>
                            </li>`)}
                        </ul>
                    `
                    : undefined}
                
                <h4>Ziel Branch auswählen</h4>
                
                ${showBranch ? html`<gw-input
                        placeholder="Zielbranch (optional, Default: HEAD)"  .value=${this.commitBranch}
                        @change=${e => this.commitBranch = e.detail.value}
                ></gw-input>` : undefined}

            </div>
            
            <div class="actions">

                ${commits.length > 0 ? html`<gw-button primary @click=${() => this.handlePush()}>Push</gw-button>` : undefined}
                ${this.commitMessage.length > 0 ? html`<gw-button primary @click=${() => this.handleCommit()}>Commit</gw-button>` : undefined}
                ${this.commitMessage.length > 0 && this.commitBranch.length > 0 ? html`<gw-button primary @click=${() => this.handleCommitPush()}>Commit & Push</gw-button>` : undefined}
                
                <gw-button type="secondary" @click=${() => this.cancel()}>Abbrechen</gw-button>
                
            </div>
        `;
    }

    cancel() {
        this.dispatchEvent(new CustomEvent('gw-project-git::cancel', {
            bubbles: true,
            composed: true
        }));
    }

    success() {
        this.dispatchEvent(new CustomEvent('gw-project-git::success', {
            bubbles: true,
            composed: true
        }));
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

    async handleCommitPush() {
        try {
            await this.handleCommit();
            await this.handlePush();
            this.success();
        } catch (error) {
            this.error = error.message;
        }
    }

    async handleCommit() {
        try {
            const response = await fetch(`/api/workspace/${this.workspaceId}/project/${this.project.id}/commit`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    message: this.commitMessage,
                    branch: this.commitBranch
                })
            });
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            this.commit = undefined
            this.commitMessage = '';
        } catch (error) {
            this.error = error.message
            throw error
        }
    }

    async handlePush() {
        try {
            const response = await fetch(`/api/workspace/${this.workspaceId}/project/${this.project.id}/push`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    branch: this.commitBranch
                })
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
        } catch (error) {
            this.error = error.message
            throw error
        }
    }

});
