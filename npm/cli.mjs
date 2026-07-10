#!/usr/bin/env node

import { readFile, writeFile } from "node:fs/promises";

import { check, fix } from "./index.js";

main().catch((error) => {
  console.error(`pre-print-check: ${error.message || error}`);
  process.exitCode = 1;
});

async function main() {
  const argv = process.argv.slice(2);
  let [command, ...rest] = argv;

  if (command === "check" || command === "fix") {
    const positional = parseCommandArgs(rest, command);
    await runCommand(command, positional);
    return;
  }

  const positional = parseCommandArgs(argv, "check");
  await runCommand("check", positional);
}

function parseCommandArgs(argv, command) {
  const options = {
    json: false,
    target: undefined,
    output: null,
    categories: [],
    unsafe: false,
  };
  const positional = [];

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--target" || arg === "-t") {
      options.target = argv[i + 1];
      i += 1;
      continue;
    }

    if (arg?.startsWith("--target=")) {
      options.target = arg.slice("--target=".length);
      continue;
    }

    if (command === "fix" && (arg === "--categories" || arg === "--fix" || arg === "-c")) {
      options.categories.push(...splitCSVArg(argv[i + 1]));
      i += 1;
      continue;
    }

    if (command === "fix" && arg?.startsWith("--categories=")) {
      options.categories.push(...splitCSVArg(arg.slice("--categories=".length)));
      continue;
    }

    if (command === "fix" && (arg === "--unsafe" || arg === "-u")) {
      options.unsafe = true;
      continue;
    }

    if (arg === "--json") {
      options.json = true;
      continue;
    }

    if (command === "fix" && (arg === "--output" || arg === "-o")) {
      options.output = argv[i + 1];
      i += 1;
      continue;
    }

    if (arg === "--help" || arg === "-h") {
      usage(command);
      process.exit(0);
    }

    if (arg.startsWith("-")) {
      throw new Error(`Unknown flag: ${arg}`);
    }

    positional.push(arg);
  }

  if (command === "fix" && options.categories.length === 0) {
    options.categories = undefined;
  }

  if (options.categories.length === 0) {
    options.categories = undefined;
  }

  return { options, positional };
}

async function runCommand(command, { options, positional }) {
  const svg = await readInput(positional);
  const opts = { target: options.target };

  if (command === "fix") {
    const fixed = await fix(svg, {
      ...opts,
      categories: options.categories,
      unsafe: options.unsafe,
    });

    if (options.output) {
      await writeFile(options.output, fixed.svg, "utf8");
    } else {
      process.stdout.write(fixed.svg);
    }
    return;
  }

  const report = await check(svg, opts);
  if (options.json) {
    console.log(JSON.stringify(report));
    return;
  }

  console.log(report.friendlySummary || "No issues");
  for (const issue of report.issues) {
    const category = issue.fixCategory ? ` (${issue.fixCategory})` : "";
    console.log(`[${issue.severity}] ${issue.code}${category}: ${issue.message}`);
  }
}

async function readInput(positional) {
  const [inputPath] = positional;

  if (inputPath) {
    return readFile(inputPath, "utf8");
  }

  if (process.stdin.isTTY) {
    usage("check");
    throw new Error("No input provided");
  }

  const chunks = [];
  for await (const chunk of process.stdin) {
    chunks.push(chunk);
  }

  return Buffer.concat(chunks).toString("utf8");
}

function splitCSVArg(raw) {
  return raw ? raw.split(",").map((item) => item.trim()).filter(Boolean) : [];
}

function usage(command) {
  const isFix = command === "fix";
  const header = isFix
    ? "Usage: pre-print-check fix [--target <target>] [--categories <csv>] [--unsafe] -o <out.svg> <input.svg>"
    : "Usage: pre-print-check [check] [--target <target>] [--json] <input.svg>";
  const examples = isFix
    ? "  pre-print-check fix --target paper --categories metadata,bleed -o art.fixed.svg art.svg"
    : "  pre-print-check --target vinyl --json art.svg";
  console.log(`${header}\n${examples}`);
}
