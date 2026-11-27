import assert from "node:assert/strict";
import { registerHooks } from "node:module";
import test from "node:test";

// Pi supplies TypeBox when loading extensions. This test uses a mock host and no Pi process.
const typebox = `data:text/javascript,${encodeURIComponent(`export const Type = {
  String: (options) => ({type: "string", ...options}),
  Object: (properties) => ({type: "object", properties}),
  Array: (items, options) => ({type: "array", items, ...options}),
}`)}`;
const hook = registerHooks({
	resolve(specifier, context, next) {
		return specifier === "typebox" ? { url: typebox, shortCircuit: true } : next(specifier, context);
	},
});
const { default: install } = await import("./bailu-ask-user.ts");
hook.deregister();

let tool;
install({ registerTool(value) { tool = value; } });

const request = {
	question: "报告要交付哪种格式？",
	options: [
		{ label: "Excel", impact: "便于复核和继续编辑" },
		{ label: "PDF", impact: "保留打印版式" },
	],
};

function execute(ui, params = request) {
	return tool.execute("test", params, undefined, undefined, { hasUI: ui !== undefined, ui });
}

test("registers one sequential question, 2–3 answers, and leaves recommendation to the UI", async () => {
	assert.equal(tool.name, "bailu_ask_user");
	assert.equal(tool.executionMode, "sequential");
	assert.equal(tool.parameters.properties.options.minItems, 2);
	assert.equal(tool.parameters.properties.options.maxItems, 3);
	const result = await execute({
		async select(question, options, dialogue) {
			assert.equal(question, request.question);
			assert.deepEqual(options, ["1. Excel（推荐） · 便于复核和继续编辑", "2. PDF · 保留打印版式", "自行填写"]);
			assert.equal(dialogue.blockedKind, "question");
			return options[1];
		},
	});
	assert.deepEqual(result.details, { status: "answered", answer: "PDF", option: 2 });
});

test("provides a free-text answer without assigning a choice number", async () => {
	const result = await execute({
		async select(_question, options) { return options.at(-1); },
		async input(question) { assert.equal(question, request.question); return "  Word 和 Excel  "; },
	});
	assert.deepEqual(result.details, { status: "answered", answer: "Word 和 Excel" });
});

test("cancelled selection and empty custom answer never accept the recommendation", async () => {
	const selections = [
		{ async select() { return undefined; } },
		{ async select(_question, options) { return options.at(-1); }, async input() { return " "; } },
	];
	for (const ui of selections) {
		assert.deepEqual((await execute(ui)).details, { status: "cancelled" });
	}
});

test("noninteractive mode returns a readable fallback and no answer", async () => {
	const result = await execute();
	assert.deepEqual(result.details, { status: "unavailable" });
	assert.match(result.content[0].text, /报告要交付哪种格式？/);
	assert.match(result.content[0].text, /Excel（推荐）/);
	assert.match(result.content[0].text, /等待回复/);
});

test("duplicate labels and an empty question are rejected before rendering", async () => {
	const invalid = [
		{ ...request, options: [...request.options, { label: " Ｅｘｃｅｌ ", impact: "重复" }] },
		{ ...request, question: "\n\t" },
	];
	for (const input of invalid) {
		const result = await execute({ async select() { assert.fail("unexpected UI request"); } }, input);
		assert.equal(result.isError, true);
		assert.deepEqual(result.details, { status: "invalid" });
	}
});
