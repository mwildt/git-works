import { LitElement, html, css } from "./lit.js";

export class DragPanel extends LitElement {
    static properties = {
        title: { type: String },
        position: { state: true },
    };

    static styles = css`
    :host {
      position: fixed;
      top: 20px;
      right: 20px;
      z-index: 2147483647;
      /* Host ist der Drag-Körper */
      touch-action: none; /* verhindert Scroll während Touch-Drag */
      user-select: none;
      font-family: system-ui, sans-serif;
    }

    .panel {
      min-width: 240px;
      max-width: 90vw;
      max-height: 80vh;
      background: #1e1e2e;
      color: #cdd6f4;
      border: 1px solid #45475a;
      border-radius: 10px;
      box-shadow: 0 8px 32px rgba(0, 0, 0, 0.45);
      display: flex;
      flex-direction: column;
      overflow: hidden;
    }

    .handle {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 8px 12px;
      background: #313244;
      cursor: grab;
      font-size: 13px;
      font-weight: 600;
    }

    .handle:active {
      cursor: grabbing;
    }

    .body {
      padding: 12px;
      overflow: auto;
      user-select: text; /* Body-Inhalt selektierbar lassen */
      font-size: 13px;
    }

    ::slotted(*) {
      max-width: 100%;
    }
  `;

    static STORAGE_KEY = "drag-panel:position";

    constructor() {
        super();
        this.title = "Panel";
        this.position = null; // { x, y } = oben links, sonst Default (CSS :host)
        this.drag = null;
    }

    connectedCallback() {
        super.connectedCallback();
        this.restorePosition();
        // Pointer-Events global: Maus verlässt während Drag das Element
        window.addEventListener("pointermove", this.onMove);
        window.addEventListener("pointerup", this.onEnd);
        window.addEventListener("pointercancel", this.onEnd);
    }

    disconnectedCallback() {
        super.disconnectedCallback();
        window.removeEventListener("pointermove", this.onMove);
        window.removeEventListener("pointerup", this.onEnd);
        window.removeEventListener("pointercancel", this.onEnd);
    }

    restorePosition() {
        try {
            const raw = localStorage.getItem(DragPanel.STORAGE_KEY);
            if (raw) {
                const pos = JSON.parse(raw);
                if (Number.isFinite(pos.x) && Number.isFinite(pos.y)) {
                    this.applyPosition(pos.x, pos.y);
                }
            }
        } catch {
            /* Storage nicht verfügbar (z.B. DNT) — Default-Position */
        }
    }

    applyPosition(x, y) {
        // Im Viewport halten (Panel könnte größer sein als der Screen)
        const rect = this.getBoundingClientRect();
        const maxX = Math.max(0, window.innerWidth - rect.width);
        const maxY = Math.max(0, window.innerHeight - rect.height);
        this.position = {
            x: Math.min(Math.max(0, x), maxX),
            y: Math.min(Math.max(0, y), maxY),
        };
        this.style.left = `${this.position.x}px`;
        this.style.top = `${this.position.y}px`;
        this.style.right = "auto"; // CSS-Default (right:20px) überschreiben
    }

    onPointerDown = (e) => {
        // Nur linke Maustaste / Touch, kein Drag aus dem Body-Inhalt heraus
        if (e.button !== undefined && e.button !== 0) return;
        this.drag = {
            startX: e.clientX,
            startY: e.clientY,
            originX: this.position?.x ?? this.getBoundingClientRect().left,
            originY: this.position?.y ?? this.getBoundingClientRect().top,
        };
        // Pointer-Capture verhindert, dass Text-Selection den Drag abbricht
        e.target.setPointerCapture?.(e.pointerId);
        e.preventDefault();
    };

    onMove = (e) => {
        if (!this.drag) return;
        const dx = e.clientX - this.drag.startX;
        const dy = e.clientY - this.drag.startY;
        this.applyPosition(this.drag.originX + dx, this.drag.originY + dy);
    };

    onEnd = () => {
        if (!this.drag) return;
        this.drag = null;
        try {
            localStorage.setItem(
                DragPanel.STORAGE_KEY,
                JSON.stringify(this.position)
            );
        } catch {
            /* ignore */
        }
    };

    firstUpdated() {
        // Falls noch keine gespeicherte Position existiert: aktuelle
        // CSS-Position übernehmen, damit der erste Drag von dort startet.
        if (!this.position) {
            const rect = this.getBoundingClientRect();
            this.applyPosition(rect.left, rect.top);
        }
    }

    render() {
        return html`
      <div class="panel">
        <div class="handle" @pointerdown="${this.onPointerDown}">
          <span>${this.title}</span>
        </div>
        <div class="body">
          <slot></slot>
        </div>
      </div>
    `;
    }
}
customElements.define("drag-panel", DragPanel);


