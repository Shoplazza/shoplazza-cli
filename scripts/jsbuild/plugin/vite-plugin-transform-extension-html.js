const fs = require('fs');
const path = require('path');
const j = require('jscodeshift');

const EXTENSION_RENDER_FN_NAME = 'extend';
const HTML_FILE_PATH_KEY = 'component';
const ASTNodeType = {
  RETURN_STATMENT: 'ReturnStatement',
  CALL_EXPRESSION: 'CallExpression',
  OBJECT_EXPRESSION: 'ObjectExpression',
  IDENTIFIER: 'Identifier',
  IMPORT_DEFAULT_SPECIFIER: 'ImportDefaultSpecifier',
  IMPORT_DECLARATION: 'ImportDeclaration'
};
const findRenderFnList = (code) => {
  const renderFnList = [];

  j(code)
    .find(j.CallExpression)
    .forEach((astNode) => {
      if (astNode?.value?.callee?.name === EXTENSION_RENDER_FN_NAME) {
        const objectExpressionNode = astNode?.value?.arguments?.find(
          (args) => args.type === ASTNodeType.OBJECT_EXPRESSION
        );
        if (objectExpressionNode) {
          const componentProp = objectExpressionNode?.properties?.find(
            (prop) => prop?.key?.name === HTML_FILE_PATH_KEY
          );

          const name = componentProp?.value?.callee?.name;
          name && renderFnList.push(name);
        }
      }
    });

  return renderFnList;
};

const findImportDeclareStatementNodeList = (code) => {
  const importDeclareStatementList = [];

  j(code)
    .find(j.ImportDeclaration)
    .forEach((importStatementNode) => {
      const importDefaultSpecifier = importStatementNode?.value?.specifiers?.find(
        (specifierNode) => specifierNode?.type === ASTNodeType.IMPORT_DEFAULT_SPECIFIER
      )?.local?.name;
      const importPath = importStatementNode?.value?.source?.value;

      importDefaultSpecifier &&
        importPath &&
        typeof importPath === 'string' &&
        importDeclareStatementList.push({ importDefaultSpecifier, importPath });
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

const createTemplateLiteral = (code) => j.stringLiteral(code);

const removeImportDeclareStatment = (code, specifiers) => {
  return j(code)
    .find(j.Program)
    .forEach((program) => {
      const body = program?.value?.body || [];
      if (body.length > 0) {
        const index = body.findIndex(
          (node) =>
            node?.type === ASTNodeType.IMPORT_DECLARATION &&
            !!node?.specifiers?.find(
              (specifier) =>
                specifier?.type === ASTNodeType.IMPORT_DEFAULT_SPECIFIER &&
                specifiers.includes(specifier?.local?.name || '')
            )
        );
        if (index > -1) program.value.body.splice(index, 1);
      }
    })
    .toSource();
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

        const renderFnList = findRenderFnList(code);
        const importDeclareStatementList = findImportDeclareStatementNodeList(code);

        const removedImportSpecifiers = [];

        const returnStatementChangedCode = j(code)
          .find(j.FunctionDeclaration)
          .forEach((functionDeclareNode) => {
            const fnName = functionDeclareNode?.value?.id?.name || '';
            if (!renderFnList.includes(fnName)) return;

            const returnStatment = functionDeclareNode?.value?.body?.body?.find(
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
                const templateLiteral = createTemplateLiteral(result);

                returnStatment.argument = templateLiteral;

                removedImportSpecifiers.push(identifierName);
              } catch (e) {
                console.error(e);
              }
            }
          })
          .toSource();

        const importStatementRemovedCode = removeImportDeclareStatment(
          returnStatementChangedCode,
          removedImportSpecifiers
        );
        return importStatementRemovedCode;
      },
      buildEnd: () => {
        console.log('transform html end');
      }
    };
  }
};
