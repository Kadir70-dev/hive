import { z } from "zod";
import { AREAS, INTERESTS } from "./hive-i18n";

export const memberSignupSchema = z
  .object({
    first_name: z.string().trim().min(1).max(60),
    email: z
      .string()
      .trim()
      .email()
      .max(254)
      .transform((email) => email.toLowerCase()),
    password: z.string().min(8).max(128),
    phone: z.string().trim().max(30),
    area: z.enum(AREAS),
    interests: z.array(z.enum(INTERESTS)).min(1).max(5),
    agreed: z.literal(true),
    lang: z.enum(["en", "ar"]),
  })
  .strict();

export type MemberSignup = z.infer<typeof memberSignupSchema>;
