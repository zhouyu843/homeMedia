document.addEventListener("DOMContentLoaded", () => {
    document.querySelectorAll("form[data-confirm-message]").forEach((form) => {
        form.addEventListener("submit", (event) => {
            const message = form.getAttribute("data-confirm-message");
            if (!message) {
                return;
            }

            if (!window.confirm(message)) {
                event.preventDefault();
            }
        });
    });
});