import { LitElement, html, css } from '../../lit.js';


customElements.define('gw-modal', class extends LitElement {

    static styles = css`
        :host {
            display: block;
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
        }
        .overlay {
            position: fixed;
            inset: 0;
            background: rgba(0, 0, 0, 0.5);
            display: grid;
            place-items: center;
            padding: 2rem 0;
            overflow: hidden;
        }
        .panel {
            background: white;
            border-radius: 12px;
            padding: 24px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
            width: min(480px, calc(100vw - 2rem));
            max-height: 100%;
            overflow-y: auto;
        }
    `;

    render() {
        return html`
            <div class="overlay" ?hidden=${!this.open}>
                <div class="panel" role="dialog" aria-modal="true">
                    <slot></slot>
                </div>
            </div>
        `;
    }

});