"use client"

import * as React from "react"
import { UploadCloudIcon, XIcon, AlertCircleIcon, CheckCircle2Icon } from "lucide-react"
import { cn } from "@/lib/utils"

// ── Types ─────────────────────────────────────────────────────────────────────

interface MediaUploadProps {
  accept?: string
  maxSizeMb?: number
  status: "idle" | "uploading" | "success" | "error"
  progress?: number
  previewUrl?: string | null
  onFileSelect: (file: File) => void
  onClear?: () => void
  className?: string
  disabled?: boolean
  label?: string
  hint?: string
}

// ── Component ─────────────────────────────────────────────────────────────────

export function MediaUpload({
  accept = "image/*",
  maxSizeMb = 5,
  status,
  progress = 0,
  previewUrl,
  onFileSelect,
  onClear,
  className,
  disabled = false,
  label = "Upload image",
  hint,
}: MediaUploadProps) {
  const inputRef = React.useRef<HTMLInputElement>(null)
  const [isDragOver, setIsDragOver] = React.useState(false)
  const [localPreview, setLocalPreview] = React.useState<string | null>(null)
  const [sizeError, setSizeError] = React.useState<string | null>(null)

  const previewSrc = localPreview ?? previewUrl

  function handleFile(file: File) {
    if (file.size > maxSizeMb * 1024 * 1024) {
      setSizeError(`File must be smaller than ${maxSizeMb}MB`)
      return
    }
    setSizeError(null)
    const reader = new FileReader()
    reader.onload = (e) => setLocalPreview(e.target?.result as string)
    reader.readAsDataURL(file)
    onFileSelect(file)
  }

  function handleInputChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (file) handleFile(file)
    e.target.value = ""
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault()
    setIsDragOver(false)
    if (disabled) return
    const file = e.dataTransfer.files?.[0]
    if (file) handleFile(file)
  }

  function handleClear() {
    setLocalPreview(null)
    setSizeError(null)
    onClear?.()
  }

  const hintText = hint ?? `Drag & drop or click to upload. ${accept.replace("image/*", "Images")} up to ${maxSizeMb}MB.`

  return (
    <div className={cn("space-y-2", className)}>
      <div
        role="button"
        tabIndex={disabled ? -1 : 0}
        aria-label={label}
        onClick={() => !disabled && inputRef.current?.click()}
        onKeyDown={(e) => {
          if (!disabled && (e.key === "Enter" || e.key === " ")) {
            e.preventDefault()
            inputRef.current?.click()
          }
        }}
        onDragOver={(e) => { e.preventDefault(); if (!disabled) setIsDragOver(true) }}
        onDragLeave={() => setIsDragOver(false)}
        onDrop={handleDrop}
        className={cn(
          "relative flex min-h-32 cursor-pointer flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          isDragOver ? "border-primary bg-primary/5" : "border-border hover:border-primary/50 hover:bg-muted/40",
          disabled && "cursor-not-allowed opacity-50",
          status === "error" && "border-destructive",
          status === "success" && "border-green-500",
        )}
      >
        {previewSrc ? (
          <div className="relative w-full overflow-hidden rounded-lg">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={previewSrc}
              alt="Preview"
              className="h-36 w-full object-contain"
            />
            {!disabled && onClear && (
              <button
                type="button"
                aria-label="Remove image"
                onClick={(e) => { e.stopPropagation(); handleClear() }}
                className="absolute right-2 top-2 flex size-6 items-center justify-center rounded-full bg-background/80 shadow hover:bg-background"
              >
                <XIcon className="size-3.5" />
              </button>
            )}
          </div>
        ) : (
          <>
            <div className="flex size-10 items-center justify-center rounded-full bg-muted">
              {status === "error" ? (
                <AlertCircleIcon className="size-5 text-destructive" />
              ) : status === "success" ? (
                <CheckCircle2Icon className="size-5 text-green-600" />
              ) : (
                <UploadCloudIcon className="size-5 text-muted-foreground" />
              )}
            </div>
            <div className="text-center">
              <p className="text-sm font-medium text-foreground">{label}</p>
              <p className="mt-0.5 text-xs text-muted-foreground">{hintText}</p>
            </div>
          </>
        )}

        {/* Progress overlay */}
        {status === "uploading" && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 rounded-xl bg-background/80 backdrop-blur-sm">
            <div className="h-1.5 w-32 overflow-hidden rounded-full bg-muted">
              <div
                className="h-full bg-primary transition-all duration-150"
                style={{ width: `${progress}%` }}
                role="progressbar"
                aria-valuenow={progress}
                aria-valuemin={0}
                aria-valuemax={100}
              />
            </div>
            <p className="text-xs text-muted-foreground">{progress}%</p>
          </div>
        )}
      </div>

      {sizeError && (
        <p className="text-xs text-destructive" role="alert">{sizeError}</p>
      )}

      <input
        ref={inputRef}
        type="file"
        accept={accept}
        className="sr-only"
        aria-hidden="true"
        tabIndex={-1}
        onChange={handleInputChange}
        disabled={disabled}
      />
    </div>
  )
}
