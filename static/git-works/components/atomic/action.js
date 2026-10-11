import { LitElement, html, css } from '../../lit.js';

customElements.define('gw-action', class extends LitElement {

    static styles = css`
        :host {
            display: inline-block;
            cursor: pointer;
            font-weight: 600;
            transition: background-color 0.15s ease, color 0.15s ease;
        }
        
        :host(:focus-visible) {
            outline: 2px solid currentcolor;
            outline-offset: 2px;
        }
        
        :host(:hover) {
            background-color: rgba(0, 0, 0, 0.08);
        }

        :host(:active) {
            background-color: rgba(0, 0, 0, 0.15);
        }
        
        :host(:focus-visible) {
            outline: 2px solid currentcolor;
            outline-offset: 2px;
        }
    `;

    constructor() {
        super();
    }

    render() {
        return html`
           <slot></slot>
        `;
    }
});