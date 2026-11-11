import { CircleCheck, CircleX, KeyRound } from 'lucide-react'
import { useId, useState } from 'react'
import { ProblemError, type ApiClient } from './api'
import { BrandGlyph } from './components/BrandGlyph'
import { characters, MIN_PASSWORD } from './components/ui'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/**
 * The page an emailed reset link opens (/reset-password?token=…): the member chooses a new password, then signs in
 * with it. The token alone identifies them; a spent or expired link is said so, with the way to get another.
 */
export function ResetPassword({ api, token, onDone }: { api: ApiClient; token: string; onDone: () => void }) {
  const id = useId()
  const [next, setNext] = useState('')
  const [again, setAgain] = useState('')
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)
  const [problem, setProblem] = useState<{ title: string; detail?: string | undefined } | null>(null)
  const short = next.length > 0 && characters(next) < MIN_PASSWORD
  const mismatch = again.length > 0 && again !== next
  const blocked = busy || !token || !next || short || again !== next
  return (
    <main id="main" className="grid min-h-svh place-items-center bg-background p-6">
      <Card className="w-full max-w-[460px] shadow-float">
        <CardHeader>
          <p className="mb-2 flex items-center gap-2 text-sm font-medium text-muted-foreground"><span className="flex size-6 items-center justify-center rounded-md bg-primary text-primary-foreground"><BrandGlyph className="size-4" /></span> 4draws back office</p>
          <CardTitle className="text-xl">Choose a new password</CardTitle>
          <CardDescription>{token ? 'From the link in your email. Your other sessions are signed out when it changes.' : 'This link is incomplete. Open the one in your email again, or ask for a new one from the sign-in page.'}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          {done ? (
            <>
              <Alert role="status" className="border-success/40 bg-success/10 text-success [&>svg]:text-success">
                <CircleCheck />
                <AlertTitle>Your password is set.</AlertTitle>
                <AlertDescription>Sign in with it now.</AlertDescription>
              </Alert>
              <Button type="button" size="lg" className="h-11 w-full" onClick={onDone}><KeyRound aria-hidden="true" /> Go to sign-in</Button>
            </>
          ) : (
            <form
              className="grid gap-4"
              onSubmit={(e) => {
                e.preventDefault()
                if (blocked) return
                setBusy(true)
                setProblem(null)
                api.post('/v1/staff/password/reset', { token, new_password: next }).then(
                  () => { setDone(true); setBusy(false) },
                  (err: unknown) => {
                    setBusy(false)
                    if (err instanceof ProblemError && err.status === 410) setProblem({ title: 'This link has expired or was already used.', detail: 'Ask for a new one from the sign-in page; links work for 30 minutes.' })
                    else if (err instanceof ProblemError) setProblem({ title: err.title, detail: err.detail })
                    else setProblem({ title: 'The server could not be reached. Try again.' })
                  },
                )
              }}
            >
              <div className="grid gap-1.5">
                <Label htmlFor={`${id}-new`}>New password</Label>
                <Input id={`${id}-new`} name="new-password" type="password" autoComplete="new-password" required aria-invalid={short || undefined} aria-describedby={`${id}-help`} value={next} onChange={(e) => { setNext(e.target.value) }} />
                <p id={`${id}-help`} className={short ? 'text-xs text-destructive' : 'text-xs text-muted-foreground'}>
                  At least {MIN_PASSWORD} characters. A short sentence you will remember works best; not your email address, nothing common.
                </p>
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor={`${id}-again`}>New password again</Label>
                <Input id={`${id}-again`} name="new-password-again" type="password" autoComplete="new-password" required aria-invalid={mismatch || undefined} value={again} onChange={(e) => { setAgain(e.target.value) }} />
                {mismatch ? <p className="text-xs text-destructive">The two do not match.</p> : null}
              </div>
              {problem ? (
                <Alert variant="destructive">
                  <CircleX />
                  <AlertTitle>{problem.title}</AlertTitle>
                  {problem.detail ? <AlertDescription>{problem.detail}</AlertDescription> : null}
                </Alert>
              ) : null}
              <Button type="submit" size="lg" className="h-11 w-full" disabled={blocked}>{busy ? 'Saving…' : 'Set password'}</Button>
            </form>
          )}
        </CardContent>
      </Card>
    </main>
  )
}
