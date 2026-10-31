(() => {
  const root = document.documentElement;
  const toggle = document.querySelector(".theme-toggle");
  const themeColor = document.querySelector('meta[name="theme-color"]');
  const storageKey = "siphon-theme";

  let savedTheme = null;
  try {
    savedTheme = localStorage.getItem(storageKey);
  } catch {
    // The toggle still works for this page if storage is unavailable.
  }

  let theme = savedTheme === "light" || savedTheme === "dark" ? savedTheme : "light";

  const updateTheme = () => {
    root.dataset.theme = theme;
    themeColor.content = theme === "light" ? "#f5f7f3" : "#080909";
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
