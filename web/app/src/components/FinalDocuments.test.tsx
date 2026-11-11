import { fireEvent, render, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import FinalDocuments from './FinalDocuments';
it.each([['empty','No returned documents'],['absent','Returned document declaration not recorded'],['null','Returned document declaration is null'],['unsupported','Returned document declaration is unsupported'],['malformed','Returned document declaration is malformed'],['pending','Returned documents pending']] as const)('CV-015 CV-016 distinguishes %s',(state,label)=>{
 render(<FinalDocuments tenant="tenant" job="job" terminal manifest={{job_id:'job',state,documents:[]}} onRetry={vi.fn()}/>);
 expect(screen.getByText(label)).toBeInTheDocument();
});
it('CV-016 Retry refreshes the conversation',()=>{
 const retry=vi.fn();
 render(<FinalDocuments tenant="tenant" job="job" terminal error="denied" onRetry={retry}/>);
 fireEvent.click(screen.getByRole('button',{name:'Retry'}));
 expect(retry).toHaveBeenCalledOnce();
});
it('CV-019 retains unresolved siblings without invented links',()=>{
 render(<FinalDocuments tenant="tenant" job="job" terminal onRetry={vi.fn()} manifest={{job_id:'job',state:'present',documents:[{alias:'remote',resolution:'external',ref:{}},{alias:'inline',resolution:'value',ref:{}},{alias:'lost',resolution:'unresolved',ref:{},diagnostic:'Original producer not recorded'}]}}/>);
 expect(screen.getByText('Original producer not recorded')).toBeInTheDocument();
 expect(screen.queryByRole('link')).not.toBeInTheDocument();
});
it('CV-015 hides the ending before completion',()=>{
 render(<FinalDocuments tenant="tenant" job="job" terminal={false} onRetry={vi.fn()}/>);
 expect(screen.queryByLabelText('Returned documents')).not.toBeInTheDocument();
});
it('CV-016 distinguishes initial loading and unavailable observation',()=>{
 const view=render(<FinalDocuments tenant="tenant" job="job" terminal onRetry={vi.fn()}/>);
 expect(screen.getByText('Loading returned documents…')).toBeInTheDocument();
 view.rerender(<FinalDocuments tenant="tenant" job="job" terminal error="Observation unavailable" onRetry={vi.fn()}/>);
 expect(screen.getByText('Returned documents unavailable')).toBeInTheDocument();
});
