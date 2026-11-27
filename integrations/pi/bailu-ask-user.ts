import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";

const Parameters = Type.Object({
	question: Type.String({ minLength: 1, maxLength: 160, description: "一条需要用户回答的问题。" }),
	options: Type.Array(Type.Object({
		label: Type.String({ minLength: 1, maxLength: 24, description: "简短、互斥的选项名。" }),
		impact: Type.String({ minLength: 1, maxLength: 90, description: "选择这一项的实际影响，用一句话说明。" }),
	}), { minItems: 2, maxItems: 3, description: "把推荐项放在第一项。界面会标记推荐，并额外提供自行填写。" }),
});

const customChoice = "自行填写";

function oneLine(value: string): string {
	return value.replace(/[\u0000-\u001f\u007f-\u009f\s]+/gu, " ").trim();
}

export default function bailuAskUser(pi: ExtensionAPI) {
	pi.registerTool({
		name: "bailu_ask_user",
		label: "询问用户",
		description: "请用户决定一个会实际改变结果的问题。先查可自查的信息，把推荐项放第一，并说明各选项的影响。",
		promptGuidelines: [
			"仅在关键决策仍需用户回答时使用。已给出的偏好、可检索的事实和可逆细节可直接处理。",
			"每次只问一个问题。只有真实答复才能作为决定。若界面不可用，用普通对话显示问题与选项并等待回复。",
		],
		parameters: Parameters,
		executionMode: "sequential",
		async execute(_id, params, signal, _onUpdate, ctx) {
			const question = oneLine(params.question);
			const options = params.options.map(({ label, impact }) => ({ label: oneLine(label), impact: oneLine(impact) }));
			const keys = new Set(options.map(({ label }) => label.normalize("NFKC").toLowerCase()));
			if (!question || options.length < 2 || options.length > 3 || keys.size !== options.length ||
				keys.has(customChoice) || options.some(({ label, impact }) => !label || !impact)) {
				return {
					content: [{ type: "text", text: "请提供一个简短的问题和 2 至 3 个不同选项，每项写明影响。" }],
					details: { status: "invalid" },
					isError: true,
				};
			}
			const labels = options.map(({ label, impact }, index) =>
				`${index + 1}. ${label}${index === 0 ? "（推荐）" : ""} · ${impact}`);
			if (!ctx.hasUI) {
				return {
					content: [{ type: "text", text: `当前模式无法弹出选择，请在本轮回复原样展示以下问题及选项并等待回复。\n${question}\n${labels.join("\n")}\n也可以自行填写。` }],
					details: { status: "unavailable" },
				};
			}

			const dialogue = { signal, blockedKind: "question" as const };
			const selected = await ctx.ui.select(question, [...labels, customChoice], dialogue);
			const option = labels.indexOf(selected ?? "");
			const answer = selected === customChoice
				? (await ctx.ui.input(question, "填写你的答案", dialogue))?.trim()
				: options[option]?.label;
			if (!answer || signal?.aborted) {
				return {
					content: [{ type: "text", text: "用户没有提交答案。请等待明确答复。" }],
					details: { status: "cancelled" },
				};
			}
			return {
				content: [{ type: "text", text: `用户答复为「${answer}」。` }],
				details: { status: "answered", answer, ...(option >= 0 ? { option: option + 1 } : {}) },
			};
		},
	});
}
