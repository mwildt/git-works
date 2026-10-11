import './lit.js'
import './drag-panel.js'
import './gw-mixin.js'


(function () {
    function mount() {
        const el = document.createElement('gw-mixin');
        document.body.appendChild(el);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', mount);
    } else {
        mount();
    }
})();
