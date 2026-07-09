"use strict";

const loadModule = () => import("./index.js");

exports.loadPrePrintCheck = async (...args) => (await loadModule()).loadPrePrintCheck(...args);
exports.check = async (...args) => (await loadModule()).check(...args);
exports.overlay = async (...args) => (await loadModule()).overlay(...args);
exports.fix = async (...args) => (await loadModule()).fix(...args);
exports.fixCategories = async (...args) => (await loadModule()).fixCategories(...args);
