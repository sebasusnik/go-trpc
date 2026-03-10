import { useEffect, useState } from "react";
import { type Highlighter, createHighlighter } from "shiki";

type Props = {
  code: string;
  lang: string;
};

// Single shared highlighter instance — loads theme + grammars once
let highlighter: Highlighter | null = null;
let highlighterPromise: Promise<Highlighter> | null = null;
const loadedLangs = new Set<string>();

export function getHighlighter(lang: string): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighter({
      themes: ["one-dark-pro"],
      langs: [lang],
    }).then((h) => {
      highlighter = h;
      loadedLangs.add(lang);
      return h;
    });
  }
  return highlighterPromise.then(async (h) => {
    if (!loadedLangs.has(lang)) {
      await h.loadLanguage(lang as Parameters<typeof h.loadLanguage>[0]);
      loadedLangs.add(lang);
    }
    return h;
  });
}

// Simple cache: code+lang → html
const htmlCache = new Map<string, string>();

export default function CodeBlock({ code, lang }: Props) {
  const cacheKey = `${lang}:${code}`;
  const cached = htmlCache.get(cacheKey);
  const [html, setHtml] = useState<string>(cached ?? "");

  useEffect(() => {
    if (cached) {
      setHtml(cached);
      return;
    }

    let cancelled = false;

    getHighlighter(lang).then((h) => {
      const result = h.codeToHtml(code, {
        lang,
        theme: "one-dark-pro",
      });
      htmlCache.set(cacheKey, result);
      if (!cancelled) setHtml(result);
    });

    return () => {
      cancelled = true;
    };
  }, [code, lang, cacheKey, cached]);

  if (!html) {
    return (
      <div className="overflow-auto h-full">
        <pre className="whitespace-pre text-zinc-300 text-sm p-4 min-w-fit">
          {code}
        </pre>
      </div>
    );
  }

  return (
    <div
      className="[&_pre]:!bg-transparent [&_pre]:p-4 [&_pre]:text-sm [&_pre]:min-w-fit [&_code]:text-sm overflow-auto h-full"
      // biome-ignore lint/security/noDangerouslySetInnerHtml: required for Shiki syntax highlighting output
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
