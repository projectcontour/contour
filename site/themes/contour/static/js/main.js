"use strict";

function mobileNavToggle() {
    var menu = document.getElementById("mobile-menu").parentElement;
    menu.classList.toggle('mobile-menu-visible');
}

function docsVersionToggle() {
    var menu = document.getElementById("dropdown-menu");
    menu.classList.toggle('dropdown-menu-visible');
}