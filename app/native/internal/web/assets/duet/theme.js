// Applies the saved theme before first paint (CSP forbids inline scripts).
(function(){var T={gold:"#0d0c0a",blue:"#f5f8fc"};var t=null;try{t=localStorage.getItem("neko-duet-theme")}catch(e){}
if(T[t]){document.documentElement.dataset.theme=t;document.querySelectorAll('meta[name="theme-color"]').forEach(function(m){m.content=T[t]});var c=document.querySelector('meta[name="color-scheme"]');if(c)c.content=t==="gold"?"dark":"light"}})();
