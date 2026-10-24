import { z } from "zod";

const chatMessageSchema = z.discriminatedUnion("role", [
  z.object({ role: z.literal("user"), content: z.string().trim().min(1).max(600) }).strict(),
  z.object({ role: z.literal("assistant"), content: z.string().trim().min(1).max(4000) }).strict(),
]);

// The complete page-session transcript travels with each request; nothing is stored.
export const hiveChatSchema = z
  .object({
    messages: z.array(chatMessageSchema).min(1).max(41),
    lang: z.enum(["en", "ar"]).default("en"),
  })
  .strict()
  .superRefine(({ messages }, ctx) => {
    if (
      messages.some(
        (message, index) => message.role !== (index % 2 === 0 ? "user" : "assistant"),
      ) ||
      messages.length % 2 !== 1
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: "Send complete question-answer pairs followed by your question.",
      });
    }
    if (messages.reduce((length, message) => length + message.content.length, 0) > 60000) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, message: "Please start a fresh conversation." });
    }
  });
