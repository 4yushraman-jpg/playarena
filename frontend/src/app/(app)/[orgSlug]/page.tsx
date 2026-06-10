import { PageHeader } from "@/components/ui/page-header"

interface Props {
  params: Promise<{ orgSlug: string }>
}

export default async function OrgDashboardPage({ params }: Props) {
  const { orgSlug } = await params
  return (
    <div>
      <PageHeader
        title="Dashboard"
        description={`Overview for /${orgSlug}`}
      />
      <p className="text-sm text-muted-foreground">Dashboard coming soon.</p>
    </div>
  )
}
