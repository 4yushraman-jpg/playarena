import { redirect } from "next/navigation"

interface Props {
  params: Promise<{ orgSlug: string }>
}

export default async function SettingsPage({ params }: Props) {
  const { orgSlug } = await params
  redirect(`/${orgSlug}/settings/profile`)
}
