import { useId } from 'react'

export default function Input({ label, error, id, className = '', ...props }) {
  const generatedId = useId()
  const inputId = id || generatedId
  const errorId = `${inputId}-error`

  return (
    <div className={`field ${className}`.trim()}>
      {label && <label htmlFor={inputId}>{label}</label>}
      <input
        id={inputId}
        className={`input ${error ? 'input-error' : ''}`.trim()}
        aria-invalid={Boolean(error)}
        aria-describedby={error ? errorId : undefined}
        {...props}
      />
      {error && (
        <small id={errorId} className="field-error">
          {error}
        </small>
      )}
    </div>
  )
}
