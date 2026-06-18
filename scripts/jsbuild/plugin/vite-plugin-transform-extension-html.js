const fs = require('fs');
const path = require('path');
const acorn = require('acorn');
const MagicString = require('magic-string');
const { htmlToJsStringLiteral } = require('./html-string-literal');

const EXTENSION_RENDER_FN_NAME = 'extend';
const HTML_FILE_PATH_KEY = 'component';
const ASTNodeType = {
  RETURN_STATMENT: 'ReturnStatement',
  CALL_EXPRESSION: 'CallExpression',
  OBJECT_EXPRESSION: 'ObjectExpression',
  IDENTIFIER: 'Identifier',
  IMPORT_DEFAULT_SPECIFIER: 'ImportDefaultSpecifier',
  IMPORT_DECLARATION: 'ImportDeclaration',
  FUNCTION_DECLARATION: 'FunctionDeclaration'
};

const parseModule = (code) => acorn.parse(code, { ecmaVersion: 'latest', sourceType: 'module' });

// Minimal pre-order ESTree walker (avoids an acorn-walk dependency). Visits every
// AST node in document order; non-node values (positions, primitives, null) are skipped.
const walk = (node, visit) => {
  if (!node || typeof node.type !== 'string') return;
  visit(node);
  for (const key in node) {
    if (key === 'start' || key === 'end' || key === 'loc' || key === 'range') continue;
    const child = node[key];
    if (Array.isArray(child)) {
      for (let i = 0; i < child.length; i++) walk(child[i], visit);
    } else if (child && typeof child === 'object') {
      walk(child, visit);
    }
  }
};

const findRenderFnList = (ast) => {
  const renderFnList = [];

  walk(ast, (node) => {
    if (node.type === ASTNodeType.CALL_EXPRESSION && node.callee && node.callee.name === EXTENSION_RENDER_FN_NAME) {
      const objectExpressionNode = (node.arguments || []).find(
        (args) => args.type === ASTNodeType.OBJECT_EXPRESSION
      );
      if (objectExpressionNode) {
        const componentProp = (objectExpressionNode.properties || []).find(
          (prop) => prop && prop.key && prop.key.name === HTML_FILE_PATH_KEY
        );

        const name = componentProp && componentProp.value && componentProp.value.callee && componentProp.value.callee.name;
        name && renderFnList.push(name);
      }
    }
  });

  return renderFnList;
};

const findImportDeclareStatementNodeList = (ast) => {
  const importDeclareStatementList = [];

  walk(ast, (node) => {
    if (node.type === ASTNodeType.IMPORT_DECLARATION) {
      const importDefaultSpecifier = (node.specifiers || []).find(
        (specifierNode) => specifierNode && specifierNode.type === ASTNodeType.IMPORT_DEFAULT_SPECIFIER
      )?.local?.name;
      const importPath = node.source && node.source.value;

      importDefaultSpecifier &&
        importPath &&
        typeof importPath === 'string' &&
        // node is retained so the import statement can be removed by source range.
        importDeclareStatementList.push({ importDefaultSpecifier, importPath, node });
    }
  });

  return importDeclareStatementList;
};

const readFile = (path) => {
  const supportedExt = ['html'];

  const ext = path.split('.').pop()?.toLowerCase() || '';
  if (supportedExt.includes(ext)) {
    return fs.readFileSync(path, {
      encoding: 'utf8'
    });
  }

  return '';
};

const removeBodyMark = (code) => {
  return code.replaceAll(/\<body\>/g, '').replaceAll(/\<\/body\>/g, '');
};

const addExtensionApiPrefix = (code) => {
  const EXTENSION_API_CALL_EXPRESSION = '__EXTENSION_UI__.extensionApi.';
  return code.replaceAll(/extensionApi\./g, EXTENSION_API_CALL_EXPRESSION);
};

let cachedHtmlStr = {};

const handleHtmlDeps = (htmlStr, curPath) => {
  let str = htmlStr;
  // eg. import "./index.html" import './index.html'
  const importReg = /import\s*(?:(?:\"([\.\/\w]+)\")|(?:'([\.\/\w]+)'))/;

  let result = importReg.exec(str);
  while (result) {
    const importedHtmlPath = result[1] || result[2];
    if (importedHtmlPath) {
      try {
        const importedHtmlAbsolutePath = path.resolve(path.dirname(curPath), importedHtmlPath);
        if (!Object.prototype.hasOwnProperty.call(cachedHtmlStr, importedHtmlAbsolutePath)) {
          cachedHtmlStr[importedHtmlAbsolutePath] = '';

          const importedHtmlStr = readFile(importedHtmlAbsolutePath);

          cachedHtmlStr[importedHtmlAbsolutePath] = handleHtmlDeps(importedHtmlStr, curPath);
        }

        str = str.replace(importReg, () => cachedHtmlStr[importedHtmlAbsolutePath]);
      } catch (e) {
        console.error(e);
      }
    }
    result = importReg.exec(str);
  }

  str = removeBodyMark(str);

  return str;
};

const strMinify = (code) => code.split('\n').filter(Boolean).join('');

const removeImportDeclareStatment = (magicString, importDeclareStatementList, specifiers) => {
  for (const stat of importDeclareStatementList) {
    if (specifiers.includes(stat.importDefaultSpecifier)) {
      magicString.remove(stat.node.start, stat.node.end);
    }
  }
};

module.exports = {
  vitePluginTransformExtensionHtml: () => {
    return {
      name: 'vitePluginTransformExtensionHtml',
      enforce: 'pre',
      apply: 'build',
      transform(code, srcPath) {
        if (!srcPath.endsWith('.js')) {
          return code;
        }

        const ast = parseModule(code);
        const renderFnList = findRenderFnList(ast);
        const importDeclareStatementList = findImportDeclareStatementNodeList(ast);

        const magicString = new MagicString(code);
        const removedImportSpecifiers = [];

        walk(ast, (functionDeclareNode) => {
          if (functionDeclareNode.type !== ASTNodeType.FUNCTION_DECLARATION) return;

          const fnName = (functionDeclareNode.id && functionDeclareNode.id.name) || '';
          if (!renderFnList.includes(fnName)) return;

          const returnStatment = (functionDeclareNode.body && functionDeclareNode.body.body || []).find(
            (node) => node.type === ASTNodeType.RETURN_STATMENT
          );

          const isIdentifierReturnType = returnStatment?.argument?.type === ASTNodeType.IDENTIFIER;
          if (!isIdentifierReturnType) return;

          const identifierName = returnStatment?.argument?.name;
          const htmlFilePathNode = importDeclareStatementList.find(
            (stat) => stat.importDefaultSpecifier === identifierName
          );

          if (htmlFilePathNode) {
            try {
              const absolutePath = path.resolve(path.dirname(srcPath), htmlFilePathNode.importPath);
              cachedHtmlStr[absolutePath] = '';
              const htmlStr = readFile(absolutePath);

              let result = handleHtmlDeps(htmlStr, srcPath);
              cachedHtmlStr = {};

              result = strMinify(result);
              result = addExtensionApiPrefix(result);

              // Replace the returned identifier (e.g. `return tpl`) with the inlined
              // HTML as a JS string literal, leaving the rest of the source untouched.
              magicString.update(
                returnStatment.argument.start,
                returnStatment.argument.end,
                htmlToJsStringLiteral(result)
              );

              removedImportSpecifiers.push(identifierName);
            } catch (e) {
              console.error(e);
            }
          }
        });

        removeImportDeclareStatment(magicString, importDeclareStatementList, removedImportSpecifiers);

        return magicString.toString();
      },
      buildEnd: () => {
        console.log('transform html end');
      }
    };
  }
};
