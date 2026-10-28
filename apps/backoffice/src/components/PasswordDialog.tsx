import { CircleCheck, CircleX } from 'lucide-react'
import { useId, useState } from 'react'
import { ProblemError, UncertainError } from '../api'
import { useSession } from '../context'
import { characters, MIN_PASSWORD } from './ui'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/**
 * A member sets their own password (PUT /v1/staff/password): the current one, the new one twice. Used from the
 * account menu, and as the only thing a member with a temporary password can do.
 */
export function PasswordForm({ onDone, forced = false }: { onDone: () => void; forced?: boolean }) {
  const { api } = useSession()
  const id = useId()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [again, setAgain] = useState('')
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<{ title: string; detail?: string | undefined } | null>(null)
  const [done, setDone] = useState(false)
  const short = next.length > 0 && characters(next) < MIN_PASSWORD
  const mismatch = again.length > 0 && again !== next
  const blocked = busy || !next || short || mismatch || again !== next
  return (
    <form
      className="grid gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (blocked) return
        setBusy(true)
        setProblem(null)
        api.put('/v1/staff/password', { current_password: current, new_password: next }).then(
          () => { setDone(true); setBusy(false); onDone() },
          (err: unknown) => {
            setBusy(false)
            if (err instanceof ProblemError) setProblem({ title: err.title, detail: err.detail })
            else if (err instanceof UncertainError) setProblem({ title: 'No answer from the server.', detail: 'Sign in again with the new password; if that fails, with the old one.' })
            else setProblem({ title: 'The server could not be reached. Try again.' })
          },
        )
      }}
    >
      <div className="grid gap-1.5">
        <Label htmlFor={`${id}-current`}>{forced ? 'Temporary password' : 'Current password'}</Label>
        <Input id={`${id}-current`} type="password" autoComplete="current-password" value={current} onChange={(e) => { setCurrent(e.target.value) }} />
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor={`${id}-new`}>New password</Label>
        <Input id={`${id}-new`} type="password" autoComplete="new-password" required aria-invalid={short || undefined} aria-describedby={`${id}-help`} value={next} onChange={(e) => { setNext(e.target.value) }} />
        <p id={`${id}-help`} className={short ? 'text-xs text-destructive' : 'text-xs text-muted-foreground'}>
          At least {MIN_PASSWORD} characters. A short sentence you will remember works best; not your email address, nothing common.
        </p>
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor={`${id}-again`}>New password again</Label>
        <Input id={`${id}-again`} type="password" autoComplete="new-password" required aria-invalid={mismatch || undefined} aria-describedby={mismatch ? `${id}-again-p` : undefined} value={again} onChange={(e) => { setAgain(e.target.value) }} />
        {mismatch ? <p id={`${id}-again-p`} className="text-xs text-destructive">The two do not match.</p> : null}
      </div>
      {problem ? (
        <Alert variant="destructive">
          <CircleX />
          <AlertTitle>{problem.title}</AlertTitle>
          {problem.detail ? <AlertDescription>{problem.detail}</AlertDescription> : null}
        </Alert>
      ) : null}
      {done ? (
        <Alert role="status" className="border-success/40 bg-success/10 text-success [&>svg]:text-success">
          <CircleCheck />
          <AlertTitle>Password changed. Your other sessions were signed out.</AlertTitle>
        </Alert>
      ) : null}
      <DialogFooter className="mt-1">
        <Button type="submit" disabled={blocked}>{busy ? 'Saving…' : 'Set password'}</Button>
      </DialogFooter>
    </form>
  )
}

export function PasswordDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) onClose() }}>
      <DialogContent className="sm:max-w-[460px]">
        <DialogHeader>
          <DialogTitle className="text-xl">Change your password</DialogTitle>
          <DialogDescription>Your other sessions are signed out when it changes.</DialogDescription>
        </DialogHeader>
        <PasswordForm onDone={() => { setTimeout(onClose, 900) }} />
      </DialogContent>
    </Dialog>
  )
}
