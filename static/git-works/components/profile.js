import { LitElement, html, css } from '../lit.js';
import './atomic/button.js';
import './atomic/input.js';
import './atomic/selectable.js';

customElements.define('gw-profile', class extends LitElement {

    static properties = {
        user: {}
    };

    static styles = css`
    `;

    constructor() {
        super();
        this.user = undefined
    }

    async connectedCallback() {
        super.connectedCallback();
        const response = await fetch('/api/user');
        this.user = await response.json()
    }

    render() {
        if (this.user === undefined) {
            return html`<h1>Dein Profil loading...</h1>`
        }

        return html`
            <h1>Dein Profil</h1>
            <pre>${JSON.stringify(this.user)}</pre>
        `
    }



});