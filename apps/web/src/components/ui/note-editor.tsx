"use client";
import { useLayoutEffect, useRef } from "react";
import { Bold, Heading2, List, Quote, Sparkles } from "lucide-react";

export function NoteEditor({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const input = useRef<HTMLTextAreaElement>(null);
  const selection = useRef<{ value: string; start: number; end: number } | null>(null);
  useLayoutEffect(() => {
    const pending = selection.current;
    if (!pending || pending.value !== value) return;
    selection.current = null;
    input.current?.focus();
    input.current?.setSelectionRange(pending.start, pending.end);
  }, [value]);
  function insert(prefix: string, suffix = "", placeholder = "Текст") {
    const el = input.current;
    const start = el?.selectionStart ?? value.length,
      end = el?.selectionEnd ?? value.length;
    const selected = value.slice(start, end) || placeholder;
    const next = value.slice(0, start) + prefix + selected + suffix + value.slice(end);
    if (next.length > 50000) return;
    // Restore selection in the same commit, before another keystroke can arrive.
    selection.current = {
      value: next,
      start: start + prefix.length,
      end: start + prefix.length + selected.length,
    };
    onChange(next);
  }
  return (
    <div className="note-editor">
      <div className="note-toolbar" role="group" aria-label="Оформление записи">
        <button type="button" aria-label="Жирный текст" onClick={() => insert("**", "**")}>
          <Bold size={18} />
        </button>
        <button
          type="button"
          aria-label="Добавить заголовок"
          onClick={() => insert("\n## ", "\n", "Главная мысль")}
        >
          <Heading2 size={19} />
        </button>
        <button
          type="button"
          aria-label="Добавить список"
          onClick={() => insert("\n- ", "\n", "Первый пункт")}
        >
          <List size={19} />
        </button>
        <button type="button" aria-label="Добавить цитату" onClick={() => insert("\n> ", "\n")}>
          <Quote size={17} />
        </button>
        <span>
          <Sparkles size={14} aria-hidden="true" /> Ваши мысли имеют значение
        </span>
      </div>
      <label className="sr-only" htmlFor="journal-entry-body">
        Текст записи
      </label>
      <textarea
        ref={input}
        id="journal-entry-body"
        value={value}
        maxLength={50000}
        placeholder="Что сегодня было важным? Начните с одной мысли…"
        onChange={(e) => onChange(e.target.value)}
      />
      <div className="note-footer">
        <span>Личная запись</span>
        <span>{value.length.toLocaleString("ru-RU")} / 50 000</span>
      </div>
    </div>
  );
}
