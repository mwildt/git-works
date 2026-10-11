import { LitElement, html, css } from '../../lit.js';

customElements.define('gw-dialog', class extends LitElement {

    static styles = css`
        :host {
            display: block;
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            margin: 20px auto;
            padding: 24px;
            background-color: white;
            border-radius: 12px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
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