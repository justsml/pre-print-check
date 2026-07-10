"use strict";

const loadModule = () => import("./check.js");

exports.loadPrePrintCheck = async (...args) => (await loadModule()).loadPrePrintCheck(...args);
exports.check = async (...args) => (await loadModule()).check(...args);
