'use client';
import { createContext, useContext, type ReactNode } from "react";
import Link from "next/link";
import { useHiveNavigate } from "@/components/hive/useHiveNavigate";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/hive-api";
import { useLang, type Lang, DICT } from "@/lib/hive-i18n";

type Ctx = { lang: Lang; t: (typeof DICT)["en"] };
const LangCtx = createContext<Ctx>({ lang: "en", t: DICT.en });
export const useT = () => useContext(LangCtx);

export type Me = {
  id: string;
  first_name: string;
  email: string;
  phone: string | null;
  area: string;
  interests: string[];
  avatar_path: string | null;
  status: "pending" | "approved" | "declined";
  isAdmin: boolean;
  display_name?: string;
  email_updates?: boolean;
};

export const meQuery = { queryKey: ["me"], queryFn: async (): Promise<Me|null> => {
 try { return await api<Me>('/me') } catch { return null }
}};
export function useMe() { return useQuery(meQuery); }

const esc = (s: string) =>
  s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]!);

/** Renders a dictionary title that may contain <em>; interpolated values are escaped. */
export function RichTitle({ as: Tag = "h1", text, values = {} }: { as?: "h1" | "h2"; text: string; values?: Record<string, string> }) {
  const html = text.replace(/\{(\w+)\}/g, (_, k: string) => esc(values[k] ?? ""));
  return <Tag dangerouslySetInnerHTML={{ __html: html }} />;
}

export function HiveShell({ children, member = false }: { children: ReactNode; member?: boolean }) {
  const { lang, setLang, t } = useLang();
  const { data: me } = useMe();
  const qc = useQueryClient();
  const navigate = useHiveNavigate();

  async function signOut() {
    await qc.cancelQueries();
    qc.clear();
    await api("/logout", { method: "POST" });
    navigate({ to: "/auth", replace: true });
  }

  return (
    <LangCtx.Provider value={{ lang, t }}>
      <div className="hv" lang={lang} dir={lang === "ar" ? "rtl" : "ltr"}>
        <header className="hv-header">
          <div className="hv-bar">
            <a href="/" className="hv-brand">
              <span className="hv-mono" aria-hidden="true">H</span>
              <span>{t.brand}</span>
            </a>
            <nav className="hv-nav" aria-label="Members">
              {member && me ? (
                <>
                  <Link href="/members">{t.members}</Link>
                  {me.status === "approved" || me.isAdmin ? (
                    <Link href="/gatherings">{t.gatherings}</Link>
                  ) : null}
                  <Link href="/profile">{t.profile}</Link>
                  {(me.status === "approved" || me.isAdmin) && <><Link href="/announcements">News</Link><Link href="/partners">Partners</Link><Link href="/safety">Safety</Link></>}
                  {me.isAdmin ? <Link href="/admin">{t.admin}</Link> : null}
                  <button type="button" className="hv-link" onClick={signOut}>{t.signOut}</button>
                </>
              ) : (
                <a href="/">{t.home}</a>
              )}
              <button type="button" className="hv-lang" onClick={() => setLang(lang === "en" ? "ar" : "en")}>
                {t.langToggle}
              </button>
            </nav>
          </div>
        </header>
        <main className="hv-wrap">{children}</main>
        <footer className="hv-footer">
          <p>
            © 2026 {t.brand} · <Link href="/guidelines">{t.guidelines}</Link> · <Link href="/privacy">{t.privacy}</Link> · <Link href="/terms">Terms of Membership</Link> ·{" "}
            <a href="mailto:hello@thehivesociety.ae">hello@thehivesociety.ae</a>
          </p>
        </footer>
      </div>
    </LangCtx.Provider>
  );
}

