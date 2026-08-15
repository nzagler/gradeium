(() => {
  const saved = localStorage.getItem("gradeium-theme")
  const theme = saved === "light" || saved === "system" || saved === "dark" ? saved : "dark"
  const dark = theme === "dark" || (theme === "system" && matchMedia("(prefers-color-scheme: dark)").matches)
  document.documentElement.classList.toggle("dark", dark)
  document.documentElement.style.colorScheme = dark ? "dark" : "light"
})()
