"use client";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useForm, type DefaultValues, type FieldValues, type Path } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { ApiError } from "@/lib/api/client";
import { useUnsavedChanges } from "./unsaved-changes";

export function useValidatedForm<T extends FieldValues>({
  schema,
  defaultValues,
  fields,
  save,
}: {
  schema: z.ZodType<T, T>;
  defaultValues: DefaultValues<T>;
  fields: readonly Path<T>[];
  save: (values: T) => Promise<T | void>;
}) {
  const form = useForm<T, unknown, T>({
    resolver: zodResolver(schema),
    defaultValues,
    mode: "onBlur",
  });
  const locked = useRef(false);
  const [saving, setSaving] = useState(false);
  const focusAfterSave = useRef<Path<T> | null>(null);
  useEffect(() => {
    if (!saving && focusAfterSave.current) {
      form.setFocus(focusAfterSave.current);
      focusAfterSave.current = null;
    }
  }, [saving, form]);
  useUnsavedChanges(form.formState.isDirty || saving);
  async function onSubmit(event?: FormEvent<HTMLFormElement>) {
    event?.preventDefault();
    if (locked.current) return;
    locked.current = true;
    setSaving(true);
    try {
      form.clearErrors("root.server");
      await form.handleSubmit(async (values) => {
        try {
          const saved = await save(values);
          form.reset(saved ?? values);
        } catch (error) {
          if (error instanceof ApiError)
            for (const [key, message] of Object.entries(error.fields)) {
              const field = fields.find((name) => name === key);
              if (field) {
                form.setError(field, { type: "server", message });
                focusAfterSave.current ??= field;
              }
            }
          form.setError("root.server", {
            type: "server",
            message:
              error instanceof ApiError
                ? error.message
                : "Не удалось сохранить. Введённые данные остались в форме.",
          });
        }
      })(event);
    } finally {
      locked.current = false;
      setSaving(false);
    }
  }
  return { form, onSubmit, saving, error: form.formState.errors.root?.server?.message };
}
