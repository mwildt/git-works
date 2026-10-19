import { LitElement, html, css } from "./lit.js";

export class GwMixin extends LitElement {
    render() {
        return html`
        <drag-panel title="git-works">
            <a @click=${async e => {
                e.preventDefault();
                const response = await fetch("/.git-works/session-mode", {method: 'PATCH'})
                if (response.ok) {
                    const loc = await response.json();
                    window.location.href = loc.location;
                } else {
                    location.reload()    
                }
                
                
            }}>Zurück</a>
        </drag-panel>  
    `;
    }
}
customElements.define("gw-mixin", GwMixin);
