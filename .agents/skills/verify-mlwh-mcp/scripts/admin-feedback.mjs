#!/usr/bin/env node
// Prove the wa admin sees the reports the MCP server submitted: log in to the
// wa frontend through the account menu as the server owner, open Feedback
// from the menu, and for each expected report screenshot its card and check
// the card text contains every expected string.
//
// Usage: admin-feedback.mjs <run-dir> <expect.json>
// expect.json: [{"id": 7, "name": "stdio", "contains": ["marker", ...]}, ...]
// Writes <run-dir>/evidence/40-*.png, 41-<name>-report.{png,txt},
// admin-feedback-summary.json. Exit 0 only when every check passes.

import { createRequire } from "node:module";
import { readFileSync, writeFileSync } from "node:fs";
import { userInfo } from "node:os";
import path from "node:path";

const [run, expectPath] = process.argv.slice(2);
if (!run || !expectPath) {
    console.error("usage: admin-feedback.mjs <run-dir> <expect.json>");
    process.exit(2);
}

function readEnv(file) {
    return Object.fromEntries(
        readFileSync(file, "utf8")
            .split("\n")
            .filter((line) => line.includes("="))
            .map((line) => [
                line.slice(0, line.indexOf("=")),
                line.slice(line.indexOf("=") + 1),
            ]),
    );
}

const stack = readEnv(path.join(run, "stack.env"));
const wa = readEnv(path.join(stack.WA_RUN, "stack.env"));
const require = createRequire(path.join(wa.REPO, "frontend", "package.json"));
const { chromium } = require("@playwright/test");

const evidence = path.join(run, "evidence");
const expected = JSON.parse(readFileSync(expectPath, "utf8"));
const password = readFileSync(
    path.join(wa.STATE_HOME, ".wa-results-server.token"),
    "utf8",
).trim();
const username = userInfo().username;
const results = [];

function check(name, ok, detail) {
    results.push({ name, ok, detail });
    console.log(`${ok ? "PASS" : "FAIL"} ${name}${detail ? ` - ${detail}` : ""}`);
}

const browser = await chromium.launch(
    process.env.WA_VERIFY_CHROMIUM
        ? { executablePath: process.env.WA_VERIFY_CHROMIUM }
        : {},
);
const page = await (
    await browser.newContext({
        ignoreHTTPSErrors: true,
        viewport: { width: 1280, height: 900 },
    })
).newPage();

try {
    await page.goto(wa.FRONTEND_URL, { waitUntil: "networkidle" });
    await page.getByRole("button", { name: "Log in" }).click();
    const form = page.getByRole("form", { name: "Log in" });
    await form.locator('input[name="username"]').fill(username);
    await form.locator('input[name="password"]').fill(password);
    await form.getByRole("button", { name: "Continue" }).click();

    const account = page.getByRole("button", { name: `${username} account` });
    await account.waitFor({ timeout: 30_000 });
    await account.click();
    await page.getByRole("menuitem", { name: "Feedback" }).click();
    await page.waitForURL("**/feedback**");
    await page.getByRole("heading", { name: "Agent feedback" }).waitFor();
    await page.screenshot({
        path: path.join(evidence, "40-feedback-page.png"),
        fullPage: true,
    });

    for (const want of expected) {
        const article = page.getByRole("article", {
            name: `Feedback #${want.id}`,
        });
        await article.waitFor({ timeout: 15_000 });
        await article.scrollIntoViewIfNeeded();
        await article.screenshot({
            path: path.join(evidence, `41-${want.name}-report.png`),
        });
        const text = await article.innerText();
        writeFileSync(path.join(evidence, `41-${want.name}-report.txt`), text);
        const missing = want.contains.filter((s) => !text.includes(s));
        check(
            `admin card #${want.id} (${want.name}) shows ${want.contains.length} expected strings`,
            missing.length === 0,
            missing.length ? `missing: ${JSON.stringify(missing)}` : "",
        );
        for (const absent of want.absent ?? []) {
            check(
                `admin card #${want.id} (${want.name}) omits ${JSON.stringify(absent)}`,
                !text.includes(absent),
            );
        }
    }
} catch (error) {
    await page
        .screenshot({ path: path.join(evidence, "49-failure.png"), fullPage: true })
        .catch(() => {});
    check("browser drive", false, String(error));
} finally {
    await browser.close();
}

writeFileSync(
    path.join(evidence, "admin-feedback-summary.json"),
    `${JSON.stringify({ expected, results }, null, 2)}\n`,
);
const failed = results.filter((r) => !r.ok).length;
console.log(failed === 0 ? "ALL PASS" : `${failed} FAILED`);
process.exit(failed === 0 ? 0 : 1);
