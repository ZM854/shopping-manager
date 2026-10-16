import type {
  ShoppingItem,
  UpdateShoppingItemRequest,
} from '../../models/shoppingList';
import IconButton from '../UI/button/IconButton/IconButton';
import Checkbox from '../UI/input/Checkbox/Checkbox';
import DeleteIcon from '../UI/svg/DeleteIcon/DeleteIcon';
import EditIcon from '../UI/svg/EditIcon/EditIcon';
import cls from './ProductCard.module.css';

type ProductCardProps = {
  product: ShoppingItem;
  pending: boolean;
  editProduct: (product: ShoppingItem) => void;
  updateProduct: (id: string, data: UpdateShoppingItemRequest) => void;
  deleteProduct: (id: string) => void;
};

const ProductCard = ({
  product,
  pending,
  editProduct,
  updateProduct,
  deleteProduct,
}: ProductCardProps) => (
  <div className={cls.card}>
    <div className={cls.card_info}>
      <Checkbox
        aria-label={`Куплено: ${product.name}`}
        name="isMarked"
        checked={product.isMarked}
        disabled={pending}
        onChange={(event) =>
          updateProduct(product.id, { isMarked: event.target.checked })
        }
      />
      <div>
        <h3>{product.name}</h3>
        <span>
          {[product.quantity, product.unit].filter(Boolean).join(' ')}
        </span>
      </div>
    </div>
    <div className={cls.card_controls}>
      <IconButton
        tone="danger"
        aria-label={`Удалить ${product.name}`}
        disabled={pending}
        onClick={() => deleteProduct(product.id)}
      >
        <DeleteIcon className={cls.icon} />
      </IconButton>
      <IconButton
        aria-label={`Редактировать ${product.name}`}
        disabled={pending}
        onClick={() => editProduct(product)}
      >
        <EditIcon className={cls.icon} />
      </IconButton>
    </div>
  </div>
);

export default ProductCard;
