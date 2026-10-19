import { LitElement, html, css } from '../../lit.js';

customElements.define('gw-button', class extends LitElement {
    static properties = {
        type: { type: String, reflect: true },
        disabled: { type: Boolean, reflect: true }
    };

    static styles = css`
        :host {
            display: inline-block;
        }

        button {
            background-color: #4CAF50;
            color: white;
            border: none;
            padding: 10px 20px;
            text-align: center;
            text-decoration: none;
            display: inline-block;
            font-size: 14px;
            margin: 4px 2px;
            cursor: pointer;
            border-radius: 8px;
            transition: background-color 0.3s ease, transform 0.2s ease;
            box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
        }
        
        button:hover:not(:disabled) {
            background-color: #43A047; /* dunkleres Grün */
        }
    
        :host([primary]) button {
            background-color: #e8f5e8;
            color: #2E7D32;
        }
    
        :host([primary]) button:hover:not(:disabled) {
            background-color: #c8e6c9; /* eine Stufe dunkler als e8f5e8 */
        }
    
        :host([danger]) button {
            background-color: #fdecea;
            color: #C62828;
        }
    
        :host([danger]) button:hover:not(:disabled) {
            background-color: #f5c6c1; /* eine Stufe dunkler als fdecea */
        }

    `;

    constructor() {
        super();
        this.type = 'primary';
        this.disabled = false;
    }

    render() {
        return html`
            <button 
                ?disabled=${this.disabled}>
                <slot></slot>
            </button>
        `;
    }
});