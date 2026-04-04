(function () {
  var theme = "light";

  try {
    var stored = window.localStorage.getItem("wrssh.theme");
    if (stored === "dark" || stored === "light") {
      theme = stored;
    }
  } catch (error) {
  }

  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
})();
