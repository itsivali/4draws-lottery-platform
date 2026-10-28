import { CircleX } from 'lucide-react'
import { useId, useState } from 'react'
import { ProblemError } from '../api'
import { useSession } from '../context'
import type { Me } from '../types'
import { characters } from './ui'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/** The server's limit on a member's name (PUT /v1/staff/me/name). */
export const MAX_NAME = 80

/**
 * A member sets the name the back office shows for them in place of their email address. Asked once after the first
 * sign-in when it is empty, and offered from the account menu.
 */
export function NameForm({ onDone, label = 'Save name' }: { onDone: (me: Me) => void; label?: string }) {
  const { api, me } = useSession()
  const id = useId()
  const [name, setName] = useState(me.name)
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<{ title: string; detail?: string | undefined } | null>(null)
  const trimmed = name.trim()
  const blocked = busy || !trimmed || characters(trimmed) > MAX_NAME
  return (
    <form
      className="grid gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (blocked) return
        setBusy(true)
        setProblem(null)
        api.put<Me>('/v1/staff/me/name', { name: trimmed }).then(
          (next) => { setBusy(false); onDone({ ...me, name: next.name }) },
          (err: unknown) => {
            setBusy(false)
            if (err instanceof ProblemError) setProblem({ title: err.title, detail: err.detail })
            else setProblem({ title: 'The server could not be reached. Try again.' })
          },
        )
      }}
    >
      <div className="grid gap-1.5">
        <Label htmlFor={`${id}-name`}>Your name</Label>
        <Input id={`${id}-name`} autoComplete="name" required maxLength={MAX_NAME} aria-describedby={`${id}-help`} value={name} onChange={(e) => { setName(e.target.value) }} />
        <p id={`${id}-help`} className="text-xs text-muted-foreground">As your colleagues know you. It is shown on every action you take.</p>
      </div>
      {problem ? (
        <Alert variant="destructive">
          <CircleX />
          <AlertTitle>{problem.title}</AlertTitle>
          {problem.detail ? <AlertDescription>{problem.detail}</AlertDescription> : null}
        </Alert>
      ) : null}
      <DialogFooter className="mt-1">
        <Button type="submit" disabled={blocked}>{busy ? 'Saving…' : label}</Button>
      </DialogFooter>
    </form>
  )
}

export function NameDialog({ open, onClose, onSaved }: { open: boolean; onClose: () => void; onSaved: (me: Me) => void }) {
  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) onClose() }}>
      <DialogContent className="sm:max-w-[460px]">
        <DialogHeader>
          <DialogTitle className="text-xl">Change your name</DialogTitle>
          <DialogDescription>The back office shows it in place of your email address.</DialogDescription>
        </DialogHeader>
        {open ? <NameForm onDone={(me) => { onSaved(me); onClose() }} /> : null}
      </DialogContent>
    </Dialog>
  )
}
