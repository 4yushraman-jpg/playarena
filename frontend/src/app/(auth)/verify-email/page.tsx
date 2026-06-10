"use client"

import { Suspense } from "react"
import Link from "next/link"
import { useSearchParams, useRouter } from "next/navigation"
import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { authApi } from "@/lib/api/auth"
import { extractApiError } from "@/lib/api-error"

function VerifyEmailContent() {
  const params = useSearchParams()
  const router = useRouter()
  const token = params.get("token")
  const email = params.get("email")

  // Initialise to "verifying" immediately when a token is present so we never
  // need to call setState synchronously inside the effect body.
  const [status, setStatus] = useState<"idle" | "verifying" | "success" | "error">(
    () => (token ? "verifying" : "idle"),
  )
  const [message, setMessage] = useState("")
  const [resendLoading, setResendLoading] = useState(false)
  const [resendDone, setResendDone] = useState(false)

  // Auto-verify when token is in URL (clicked from email link)
  useEffect(() => {
    if (!token) return
    authApi
      .verifyEmail(token)
      .then(() => {
        setStatus("success")
        setTimeout(() => router.push("/login"), 2500)
      })
      .catch((err) => {
        setStatus("error")
        setMessage(extractApiError(err))
      })
  }, [token, router])

  async function handleResend() {
    if (!email || resendLoading) return
    setResendLoading(true)
    try {
      await authApi.resendVerification({ email })
      setResendDone(true)
    } finally {
      setResendLoading(false)
    }
  }

  if (token) {
    return (
      <div className="rounded-xl border border-border bg-card p-8 shadow-sm text-center space-y-4">
        {status === "verifying" && <p className="text-muted-foreground">Verifying your email…</p>}
        {status === "success" && (
          <>
            <p className="text-sm font-medium text-green-600 dark:text-green-400">
              Email verified! Redirecting to sign in…
            </p>
          </>
        )}
        {status === "error" && (
          <>
            <p className="text-sm text-destructive">{message || "Verification failed."}</p>
            <Link href="/login" className="text-sm text-primary hover:underline">
              Back to sign in
            </Link>
          </>
        )}
      </div>
    )
  }

  // No token — show "check your inbox" state
  return (
    <div className="rounded-xl border border-border bg-card p-8 shadow-sm space-y-6">
      <div className="space-y-1 text-center">
        <h1 className="text-xl font-semibold">Check your email</h1>
        <p className="text-sm text-muted-foreground">
          We sent a verification link to{" "}
          {email ? <span className="font-medium text-foreground">{email}</span> : "your email address"}.
        </p>
      </div>

      <p className="text-sm text-muted-foreground text-center">
        Didn&apos;t receive it?{" "}
        {resendDone ? (
          <span className="text-green-600 dark:text-green-400">Sent!</span>
        ) : (
          <Button
            variant="link"
            className="h-auto p-0 text-sm"
            onClick={handleResend}
            disabled={resendLoading || !email}
          >
            {resendLoading ? "Sending…" : "Resend"}
          </Button>
        )}
      </p>

      <p className="text-center text-sm text-muted-foreground">
        <Link href="/login" className="text-primary hover:underline">
          Back to sign in
        </Link>
      </p>
    </div>
  )
}

export default function VerifyEmailPage() {
  return (
    <Suspense>
      <VerifyEmailContent />
    </Suspense>
  )
}
