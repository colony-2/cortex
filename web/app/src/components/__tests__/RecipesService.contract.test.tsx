/**
 * Contract tests for RecipesService
 *
 * These tests verify that the generated OpenAPI client has the expected
 * interface. Unlike the component tests that mock RecipesService, these
 * tests import the REAL service to ensure it has all required methods.
 *
 * This would have caught the bug where createRecipe was missing from
 * the generated client after a build issue.
 */

import { describe, it, expect } from 'vitest';
import { RecipesService } from '@colony2/openapi-client';

describe('RecipesService Contract', () => {
  it('should export RecipesService', () => {
    expect(RecipesService).toBeDefined();
    expect(typeof RecipesService).toBe('function');
  });

  it('should have listRecipes static method', () => {
    expect(RecipesService.listRecipes).toBeDefined();
    expect(typeof RecipesService.listRecipes).toBe('function');
  });

  it('should have createRecipe static method', () => {
    expect(RecipesService.createRecipe).toBeDefined();
    expect(typeof RecipesService.createRecipe).toBe('function');
  });

  it('should have getRecipe static method', () => {
    expect(RecipesService.getRecipe).toBeDefined();
    expect(typeof RecipesService.getRecipe).toBe('function');
  });

  it('should have updateRecipe static method', () => {
    expect(RecipesService.updateRecipe).toBeDefined();
    expect(typeof RecipesService.updateRecipe).toBe('function');
  });

  it('should have deleteRecipe static method', () => {
    expect(RecipesService.deleteRecipe).toBeDefined();
    expect(typeof RecipesService.deleteRecipe).toBe('function');
  });

  it('should have publishRecipe static method', () => {
    expect(RecipesService.publishRecipe).toBeDefined();
    expect(typeof RecipesService.publishRecipe).toBe('function');
  });

  it('should have unpublishRecipe static method', () => {
    expect(RecipesService.unpublishRecipe).toBeDefined();
    expect(typeof RecipesService.unpublishRecipe).toBe('function');
  });

  it('should have getRecipeHistory static method', () => {
    expect(RecipesService.getRecipeHistory).toBeDefined();
    expect(typeof RecipesService.getRecipeHistory).toBe('function');
  });

  it('should have correct method signatures for createRecipe', () => {
    // Verify the function accepts the right number of parameters
    expect(RecipesService.createRecipe.length).toBe(2);
  });

  it('should have correct method signatures for updateRecipe', () => {
    expect(RecipesService.updateRecipe.length).toBe(3);
  });

  it('should have correct method signatures for publishRecipe', () => {
    expect(RecipesService.publishRecipe.length).toBe(3);
  });
});
