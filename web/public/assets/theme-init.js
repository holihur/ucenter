(function () {
  try {
    var theme = localStorage.getItem("uc_theme") || "system";
    var dark = theme === "dark" || (theme === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
    document.documentElement.classList.toggle("dark", dark);
    document.documentElement.style.colorScheme = dark ? "dark" : "light";
    var lang = localStorage.getItem("uc_lang") || "en";
    var locales = { en: "en-US", zh: "zh-CN", es: "es-ES", hi: "hi-IN", ar: "ar-SA", pt: "pt-BR", bn: "bn-BD", ru: "ru-RU", ja: "ja-JP", fr: "fr-FR" };
    document.documentElement.lang = locales[lang] || "en-US";
    document.documentElement.dir = lang === "ar" ? "rtl" : "ltr";
  } catch (e) {}
})();
