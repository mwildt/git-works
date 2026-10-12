import { LitElement, html, css } from '../../lit.js';

customElements.define('gw-badge', class extends LitElement {

    static properties = {
        ok: {},
        warn: {},
        error: {}
    }

    static styles = css`
        :host {
            display: inline-block;
            font-size: 12px;
            padding: 4px 10px;
            border-radius: 12px;
            font-weight: 600;
            white-space: nowrap;
            
            background-color: #e3f2fd;
            color: #1565C0;
        }
           :host([ok]) {
                background-color: #e8f5e8;
                color: #2E7D32;
            }
            :host([error]) {
                background-color: #fdecea;
                color: #C62828;
            }
            :host([warn]) {
                background-color: #fff3e0;
                color: #EF6C00;
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