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

  let theme = savedTheme === "light" || savedTheme === "dark" ? savedTheme : "dark";

  const updateTheme = () => {
    root.dataset.theme = theme;
    themeColor.content = theme === "light" ? "#f5f7f3" : "#111719";
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

  const navLinks = [...document.querySelectorAll('.side nav a[href^="#"]')];
  const sections = navLinks
    .map((link) => document.querySelector(link.getAttribute("href")))
    .filter(Boolean);

  if ("IntersectionObserver" in window && sections.length) {
    const visible = new Set();
    const sectionLinks = new Map(
      navLinks.map((link) => [document.querySelector(link.getAttribute("href")), link]),
    );

    const updateCurrentSection = () => {
      const current = [...visible].sort(
        (a, b) => a.getBoundingClientRect().top - b.getBoundingClientRect().top,
      )[0];
      for (const link of navLinks) link.removeAttribute("aria-current");
      if (current) sectionLinks.get(current)?.setAttribute("aria-current", "location");
    };

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) visible.add(entry.target);
          else visible.delete(entry.target);
        }
        updateCurrentSection();
      },
      { rootMargin: "-20% 0px -70% 0px" },
    );

    sections.forEach((section) => observer.observe(section));
  }
})();
