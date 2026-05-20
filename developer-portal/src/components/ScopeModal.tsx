import * as Dialog from '@radix-ui/react-dialog'
import { RefObject, useId } from 'react'
import MethodBadge from './MethodBadge'
import { t } from '../i18n'

interface Props {
  method: string;
  scopes: string[];
  path: string;
  onClose: () => void;
  restoreFocusRef?: RefObject<HTMLElement>;
}

export default function ScopeModal({ method, scopes, path, onClose, restoreFocusRef }: Props) {
  const titleId = useId()

  return (
    <Dialog.Root open onOpenChange={(open) => { if (!open) onClose() }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-slate-950/45 backdrop-blur-[2px]" />
        <Dialog.Content
          aria-labelledby={titleId}
          onCloseAutoFocus={(event) => {
            if (!restoreFocusRef?.current) {
              return
            }
            event.preventDefault()
            restoreFocusRef.current.focus()
          }}
          className="fixed inset-0 z-[60] flex items-center justify-center p-4 outline-none"
        >
          <div className="portal-card portal-card-strong w-full max-w-md rounded-[1.6rem] p-6 shadow-[0_24px_56px_rgba(15,23,42,0.22)]">
            <div className="mb-4 flex items-start justify-between gap-3">
              <div className="min-w-0">
                <Dialog.Title id={titleId} className="flex items-center gap-2 text-sm font-semibold text-slate-900">
                  <MethodBadge method={method} />
                  <code className="min-w-0 truncate text-sm text-slate-700">{path}</code>
                </Dialog.Title>
              </div>
              <Dialog.Close asChild>
                <button
                  type="button"
                  aria-label={t('detail.close')}
                  className="inline-flex min-h-[44px] min-w-[44px] items-center justify-center rounded-full border border-slate-200 bg-white text-slate-500 transition hover:border-slate-300 hover:text-slate-900"
                >
                  <span aria-hidden="true" className="text-2xl leading-none">&times;</span>
                </button>
              </Dialog.Close>
            </div>

            <div className="border-t border-slate-200 pt-4">
              <h4 className="mb-3 text-sm font-semibold text-slate-700">{t('routes.scopes')}</h4>
              {scopes.length > 0 ? (
                <div className="space-y-2">
                  {scopes.map((scope) => (
                    <div
                      key={scope}
                      className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 font-mono text-sm text-slate-800"
                    >
                      {scope}
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm italic text-slate-500">{t('routes.noScopes')}</p>
              )}
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
