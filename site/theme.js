(() => {
  const root = document.documentElement;
  const toggle = document.querySelector(".theme-toggle");
  const storageKey = "siphon-theme";
  const systemTheme = window.matchMedia("(prefers-color-scheme: light)").matches
    ? "light"
    : "dark";

  let savedTheme = null;
  try {
    savedTheme = localStorage.getItem(storageKey);
  } catch {
    // The toggle still works for this page if storage is unavailable.
  }

  let theme = savedTheme === "light" || savedTheme === "dark" ? savedTheme : systemTheme;

  const updateTheme = () => {
    root.dataset.theme = theme;
    toggle.textContent = `Switch to ${theme === "dark" ? "light" : "dark"} mode`;
    toggle.setAttribute("aria-label", `Switch to ${theme === "dark" ? "light" : "dark"} mode`);
  };

  updateTheme();
  toggle.hidden = false;
  toggle.addEventListener("click", () => {
    theme = theme === "dark" ? "light" : "dark";
    updateTheme();
    try {
      localStorage.setItem(storageKey, theme);
    } catch {
      // The selected theme remains active until this page is closed.
    }
  });
})();
