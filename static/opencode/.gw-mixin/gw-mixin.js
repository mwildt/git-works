import { LitElement, html, css } from "./lit.js";

export class GwMixin extends LitElement {
    render() {
        return html`
        <drag-panel title="git-works">
            <a @click=${async e => {
                e.preventDefault();
                await fetch("/.git-works/session-mode", {method: 'PATCH'})
                location.reload()
            }}>Zurück</a>
        </drag-panel>  
    `;
    }
}
customElements.define("gw-mixin", GwMixin);
