import type { RecipeJobSummary } from '@colony2/shared/types';
import { stringifyJobJSON } from '@colony2/shared/jobJSON';
import { cpuCores, executionRequirements, memoryQuantity } from '../utils/executionRequirements';
import './JobEnvironment.css';

function Values({ values }: { values: string[] }) {
  return <ul>{values.map((value, index) => <li key={index}>{value}</li>)}</ul>;
}
export default function JobEnvironment({ job, showHeading = true }: { job: RecipeJobSummary; showHeading?: boolean }) {
  const { values, view, retained, message } = executionRequirements(job);
  const fields = values && [
    ['Base', values.base],
    ['CPU', values.resources?.cpu ? `${cpuCores(values.resources.cpu)} cores` : undefined],
    ['Memory', values.resources?.memory ? memoryQuantity(values.resources.memory) : undefined],
    ['Ephemeral storage', values.resources?.['ephemeral-storage'] ? memoryQuantity(values.resources['ephemeral-storage']) : undefined],
    ['Legacy image', values.image],
    ['Legacy platform', values.platform],
  ].filter(([, value]) => !!value);
  return <section className="job-environment" aria-label="Execution requirements">
    {showHeading && <h2>Execution requirements</h2>}
    {message && <p>{message}</p>}
    {values && <>
      {fields?.some(([label]) => label === 'CPU' || label === 'Memory' || label === 'Ephemeral storage') && <p className="job-environment-hint">Resource values are minimum requirements.</p>}
      {((fields?.length ?? 0) > 0 || !!values.capabilities?.length) && <dl>{fields?.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}
        {!!values.capabilities?.length && <div><dt>Capabilities</dt><dd><Values values={values.capabilities} /></dd></div>}
      </dl>}
      {!!values.packages?.length && <details><summary>Packages ({values.packages.length})</summary><Values values={values.packages} /></details>}
      {values.python && <details><summary>Python environment{values.python['requires-python'] ? ` · ${values.python['requires-python']}` : ''} · {values.python.dependencies?.length || 0} dependencies</summary>
        {values.python['requires-python'] && <p>Python version: {values.python['requires-python']}</p>}
        {values.python.dependencies?.length ? <Values values={values.python.dependencies} /> : <p>No dependencies specified.</p>}
      </details>}
    </>}
    <details className="job-environment-details"><summary>Details</summary>
      <p>Source: {view?.source === 'submission' ? 'At submission' : view?.source === 'yield' ? 'Published by job' : 'Unavailable'}</p>
      <p>Resolution: {typeof view?.demand?.recipe_resolution === 'string' ? view.demand.recipe_resolution : 'Unavailable'}</p>
      <p>Interpretation: {view?.status || 'unavailable'}</p>
      {retained && <p>These stored requirements do not describe live demand or actual resource allocation.</p>}
      {typeof view?.diagnostic === 'string' && view.diagnostic && <p className="job-environment-diagnostic">{view.diagnostic}</p>}
      <pre>{stringifyJobJSON({ execution: job.execution, recorded_execution: job.recorded_execution }, 2)}</pre>
    </details>
  </section>;
}
