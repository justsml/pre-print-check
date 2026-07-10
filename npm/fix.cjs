"use strict";

const loadModule = () => import("./fix.js");

exports.loadPrePrintCheck = async (...args) => (await loadModule()).loadPrePrintCheck(...args);
exports.fix = async (...args) => (await loadModule()).fix(...args);
exports.fixCategories = async (...args) => (await loadModule()).fixCategories(...args);
